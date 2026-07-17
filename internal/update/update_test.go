package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.2.3", "1.2.3", 0},
		{"v1.2.3", "1.2.3", 0}, // leading v ignored
		{"1.2.3", "v1.2.3", 0},
		{"1.2.4", "1.2.3", 1},
		{"1.2.3", "1.2.4", -1},
		{"1.3.0", "1.2.9", 1},
		{"2.0.0", "1.9.9", 1},
		{"0.1.4", "0.1.3", 1},
		{"1.2", "1.2.0", 0},    // missing component treated as 0
		{"1", "1.0.0", 0},      // fully missing
		{"1.2.0", "1.2", 0},    // symmetric
		{"1.2.10", "1.2.9", 1}, // numeric, not lexical
		// pre-releases sort before the final of equal numbers
		{"1.2.3-rc1", "1.2.3", -1},
		{"1.2.3", "1.2.3-rc1", 1},
		{"1.2.3-rc1", "1.2.3-rc2", -1},
		{"1.2.3+build5", "1.2.3", 0}, // build metadata ignored
		{"v0.1.4", "v0.1.3", 1},
		// Adversarial/degenerate inputs: garbage must degrade to 0.0.0 and
		// compare deterministically, never panic. (A malicious or broken
		// release feed controls one side of this comparison.)
		{"", "", 0},
		{"garbage", "0.0.0", 0},
		{"0.0.0", "", 0},
		{"v", "0.0.0", 0},
		{"..", "0.0.0", 0},
		{"1.2.3-", "1.2.3", 0},                      // empty pre-release == final
		{"999999999.999999999.999999999", "1.0", 1}, // huge components don't overflow int
		{"0.10.0", "0.9.0", 1},                      // per-component numeric, not lexical
	}
	for _, tt := range tests {
		if got := compareVersions(tt.a, tt.b); got != tt.want {
			t.Errorf("compareVersions(%q,%q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestSelectAsset(t *testing.T) {
	assets := []ghAsset{
		{Name: "Helm-v0.1.3-macos-universal.app.zip", URL: "u-mac-zip"},
		{Name: "helm-v0.1.3-macos-universal.tar.gz", URL: "u-mac-tgz"},
		{Name: "helm-0.1.3-linux-amd64.tar.gz", URL: "u-lin-tgz"},
		{Name: "helm-0.1.3-linux-amd64.deb", URL: "u-lin-deb"},
		{Name: "helm-0.1.3-linux-x86_64.rpm", URL: "u-lin-rpm"},
		{Name: "SHA256SUMS-linux.txt", URL: "u-sums"},
	}
	if got := selectAsset(assets, "darwin", "arm64"); got != "u-mac-zip" {
		t.Errorf("darwin asset = %q, want u-mac-zip (.app.zip preferred)", got)
	}
	if got := selectAsset(assets, "linux", "amd64"); got != "u-lin-tgz" {
		t.Errorf("linux asset = %q, want u-lin-tgz (raw tarball preferred)", got)
	}
	// x86_64 alias for amd64
	only := []ghAsset{{Name: "helm-0.1.3-linux-x86_64.tar.gz", URL: "x"}}
	if got := selectAsset(only, "linux", "amd64"); got != "x" {
		t.Errorf("x86_64 alias not matched: %q", got)
	}
	if got := selectAsset(assets, "windows", "amd64"); got != "" {
		t.Errorf("windows asset = %q, want empty", got)
	}
}

func TestParseChecksums(t *testing.T) {
	data := "abc123  helm-0.1.3-linux-amd64.tar.gz\n" +
		"def456 *Helm-v0.1.3-macos-universal.app.zip\n" +
		"garbage line\n"
	got := parseChecksums(data)
	if got["helm-0.1.3-linux-amd64.tar.gz"] != "abc123" {
		t.Errorf("linux sum = %q", got["helm-0.1.3-linux-amd64.tar.gz"])
	}
	if got["Helm-v0.1.3-macos-universal.app.zip"] != "def456" {
		t.Errorf("mac sum (binary marker) = %q", got["Helm-v0.1.3-macos-universal.app.zip"])
	}
	if len(got) != 2 {
		t.Errorf("parsed %d entries, want 2", len(got))
	}
}

// TestParseChecksumsAdversarial: checksum files come off the network, so the
// parser must tolerate hostile or platform-mangled content — CRLF endings,
// path-prefixed names (keyed by basename so lookups still match), mixed-case
// hex (normalized), and non-hex "checksums" (dropped, so verification later
// fails closed rather than comparing against garbage).
func TestParseChecksumsAdversarial(t *testing.T) {
	data := "ABCDEF12  mixed-case.tar.gz\r\n" +
		"deadbeef  dist/subdir/pathy.tar.gz\r\n" +
		"nothexatall  bad-sum.tar.gz\n" +
		"deadbeef too many fields here\n" +
		"\n"
	got := parseChecksums(data)
	if got["mixed-case.tar.gz"] != "abcdef12" {
		t.Errorf("mixed-case hex = %q, want normalized lowercase abcdef12", got["mixed-case.tar.gz"])
	}
	if got["pathy.tar.gz"] != "deadbeef" {
		t.Errorf("path-prefixed entry = %q, want keyed by basename", got["pathy.tar.gz"])
	}
	if len(got) != 2 {
		t.Errorf("parsed %d entries, want exactly 2 (non-hex and malformed lines dropped): %v", len(got), got)
	}
}

func TestDetectChannel(t *testing.T) {
	tests := []struct {
		name, exe, appimage, goos string
		want                      Channel
	}{
		{"appimage", "/tmp/.mount_HelmXXXX/usr/bin/helm", "/home/u/Apps/Helm.AppImage", "linux", ChannelAppImage},
		{"pacman", "/usr/bin/helm", "", "linux", ChannelManaged},
		{"usr-local", "/usr/local/bin/helm", "", "linux", ChannelManaged},
		{"opt", "/opt/helm/helm", "", "linux", ChannelManaged},
		{"mac app", "/Applications/Helm.app/Contents/MacOS/helm", "", "darwin", ChannelMacApp},
		{"standalone linux", "/home/u/helm/bin/helm", "", "linux", ChannelStandalone},
		{"standalone mac binary", "/Users/u/helm/bin/helm", "", "darwin", ChannelStandalone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectChannel(tt.exe, tt.appimage, tt.goos); got != tt.want {
				t.Errorf("DetectChannel(%q,%q,%q) = %q, want %q", tt.exe, tt.appimage, tt.goos, got, tt.want)
			}
		})
	}
	if !ChannelAppImage.CanSelfReplace() || !ChannelStandalone.CanSelfReplace() {
		t.Error("appimage/standalone should be self-replaceable")
	}
	if ChannelManaged.CanSelfReplace() || ChannelMacApp.CanSelfReplace() {
		t.Error("managed/macapp must NOT be self-replaceable")
	}
}

func TestCheck(t *testing.T) {
	t.Run("newer release is offered", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"tag_name":"v0.2.0","html_url":"https://x/rel","body":"notes","assets":[{"name":"helm-0.2.0-linux-amd64.tar.gz","browser_download_url":"dl"}]}`))
		}))
		defer srv.Close()
		info, err := Check(context.Background(), srv.URL, "0.1.3")
		if err != nil {
			t.Fatal(err)
		}
		if !info.UpdateAvailable || info.LatestVersion != "v0.2.0" {
			t.Errorf("info = %+v, want update available to v0.2.0", info)
		}
	})

	t.Run("same version is not an update", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"tag_name":"v0.1.3","assets":[]}`))
		}))
		defer srv.Close()
		info, err := Check(context.Background(), srv.URL, "0.1.3")
		if err != nil || info.UpdateAvailable {
			t.Errorf("info = %+v, err = %v; want no update", info, err)
		}
	})

	t.Run("404 (no/private releases) is benign", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()
		info, err := Check(context.Background(), srv.URL, "0.1.3")
		if err != nil {
			t.Errorf("404 should be benign, got err %v", err)
		}
		if info.UpdateAvailable {
			t.Error("404 should not report an update")
		}
	})

	t.Run("rate limit is an error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		defer srv.Close()
		if _, err := Check(context.Background(), srv.URL, "0.1.3"); err == nil {
			t.Error("expected rate-limit error on 403")
		}
	})

	t.Run("malformed release JSON is an error, not a phantom update", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"tag_name": "v9.9.9", "assets": [`)) // truncated
		}))
		defer srv.Close()
		info, err := Check(context.Background(), srv.URL, "0.1.3")
		if err == nil {
			t.Error("expected a parse error for truncated release JSON")
		}
		if info.UpdateAvailable {
			t.Error("a parse failure must never report an update as available")
		}
	})

	t.Run("empty tag_name is not an update", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"tag_name":"","assets":[]}`))
		}))
		defer srv.Close()
		info, err := Check(context.Background(), srv.URL, "0.1.3")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.UpdateAvailable {
			t.Error("an empty tag_name must not be offered as an update")
		}
	})
}

func TestDownloadVerifiesChecksum(t *testing.T) {
	payload := []byte("helm-binary-bytes")

	mux := http.NewServeMux()
	mux.HandleFunc("/asset/helm-0.1.3-linux-amd64.tar.gz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	})
	// checksums served with the correct hash, filled in per-request from payload.
	var sums string
	mux.HandleFunc("/SHA256SUMS-linux.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sums))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	assetURL := srv.URL + "/asset/helm-0.1.3-linux-amd64.tar.gz"
	sumsURL := srv.URL + "/SHA256SUMS-linux.txt"
	dir := t.TempDir()

	t.Run("valid checksum downloads", func(t *testing.T) {
		sums = sha256Hex(payload) + "  helm-0.1.3-linux-amd64.tar.gz\n"
		path, err := Download(context.Background(), assetURL, sumsURL, dir)
		if err != nil {
			t.Fatalf("download err: %v", err)
		}
		got, _ := os.ReadFile(path)
		if string(got) != string(payload) {
			t.Error("downloaded content mismatch")
		}
		if filepath.Base(path) != "helm-0.1.3-linux-amd64.tar.gz" {
			t.Errorf("path = %q", path)
		}
	})

	t.Run("bad checksum fails closed", func(t *testing.T) {
		sums = "deadbeef  helm-0.1.3-linux-amd64.tar.gz\n"
		if _, err := Download(context.Background(), assetURL, sumsURL, t.TempDir()); err == nil {
			t.Error("expected checksum-mismatch error")
		}
	})

	t.Run("missing checksum refuses", func(t *testing.T) {
		sums = "abc  some-other-file.tar.gz\n"
		if _, err := Download(context.Background(), assetURL, sumsURL, t.TempDir()); err == nil {
			t.Error("expected refusal when no checksum is published for the asset")
		}
	})
}
