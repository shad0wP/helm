// Package update implements an in-app update check against GitHub Releases,
// with a hand-rolled semver comparison (zero third-party deps), asset
// selection by platform, install-channel detection, and a checksum-verified
// download.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// DefaultReleasesURL is the update source: the public GitHub Releases API for
// this repo. shad0wP/helm is a public repository, so this is a plain
// unauthenticated GET — no token is ever embedded in the distributed binary.
const DefaultReleasesURL = "https://api.github.com/repos/shad0wP/helm/releases/latest"

// httpTimeout bounds every network call (mirrors the service package's pattern).
const httpTimeout = 10 * time.Second

const (
	maxMetadataBytes = 1 << 20
	maxUpdateBytes   = 500 << 20
)

// Info is the result of an update check, shaped for the frontend.
type Info struct {
	CurrentVersion  string `json:"currentVersion"`
	LatestVersion   string `json:"latestVersion"`
	UpdateAvailable bool   `json:"updateAvailable"`
	ReleaseURL      string `json:"releaseURL"`
	ReleaseNotes    string `json:"releaseNotes"`
	AssetURL        string `json:"assetURL"`
	ChecksumsURL    string `json:"checksumsURL"`
}

type ghAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type ghRelease struct {
	TagName string    `json:"tag_name"`
	HTMLURL string    `json:"html_url"`
	Body    string    `json:"body"`
	Assets  []ghAsset `json:"assets"`
}

// splitVersion splits a version into its numeric core (major, minor, patch) and
// any pre-release suffix. A leading "v" and build metadata (+...) are stripped.
func splitVersion(v string) (nums [3]int, pre string) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	core := v
	if i := strings.IndexByte(v, '-'); i >= 0 {
		core, pre = v[:i], v[i+1:]
	}
	for i, part := range strings.SplitN(core, ".", 3) {
		nums[i], _ = strconv.Atoi(strings.TrimFunc(part, func(r rune) bool {
			return r < '0' || r > '9'
		}))
	}
	return nums, pre
}

// compareVersions returns -1 if a<b, 0 if equal, +1 if a>b. Zero deps.
// Missing components are treated as 0. A pre-release (e.g. 1.2.3-rc1) sorts
// *before* its release (1.2.3) — the conservative choice, so we never offer a
// pre-release as an "update" over an equal-numbered final.
func compareVersions(a, b string) int {
	na, pa := splitVersion(a)
	nb, pb := splitVersion(b)
	for i := 0; i < 3; i++ {
		if na[i] < nb[i] {
			return -1
		}
		if na[i] > nb[i] {
			return 1
		}
	}
	switch {
	case pa == "" && pb == "":
		return 0
	case pa == "" && pb != "":
		return 1 // a is final, b is pre-release → a is newer
	case pa != "" && pb == "":
		return -1
	default:
		return comparePrerelease(pa, pb)
	}
}

func comparePrerelease(a, b string) int {
	ap, bp := strings.Split(a, "."), strings.Split(b, ".")
	limit := len(ap)
	if len(bp) > limit {
		limit = len(bp)
	}
	for i := 0; i < limit; i++ {
		if i >= len(ap) {
			return -1
		}
		if i >= len(bp) {
			return 1
		}
		ai, aerr := strconv.ParseUint(ap[i], 10, 64)
		bi, berr := strconv.ParseUint(bp[i], 10, 64)
		switch {
		case aerr == nil && berr == nil:
			if ai < bi {
				return -1
			}
			if ai > bi {
				return 1
			}
		case aerr == nil && berr != nil:
			return -1
		case aerr != nil && berr == nil:
			return 1
		default:
			if order := strings.Compare(ap[i], bp[i]); order != 0 {
				return order
			}
		}
	}
	return 0
}

// parseRelease decodes a GitHub release JSON payload. Pure; unit-tested.
func parseRelease(data []byte) (ghRelease, error) {
	var r ghRelease
	if err := json.Unmarshal(data, &r); err != nil {
		return ghRelease{}, fmt.Errorf("parsing release JSON: %w", err)
	}
	return r, nil
}

// selectAsset picks the best download asset for the given platform. Pure;
// unit-tested. Returns "" when nothing matches.
func selectAsset(assets []ghAsset, goos, goarch string) string {
	match := func(pred func(name string) bool) string {
		for _, a := range assets {
			if pred(strings.ToLower(a.Name)) {
				return a.URL
			}
		}
		return ""
	}
	switch goos {
	case "darwin":
		// Prefer the app bundle zip, then any macOS/darwin asset.
		if u := match(func(n string) bool {
			return (strings.Contains(n, "macos") || strings.Contains(n, "darwin")) && strings.HasSuffix(n, ".app.zip")
		}); u != "" {
			return u
		}
		return match(func(n string) bool {
			return strings.Contains(n, "macos") || strings.Contains(n, "darwin")
		})
	case "linux":
		arches := []string{goarch}
		if goarch == "amd64" {
			arches = append(arches, "x86_64")
		}
		// Prefer the channel-agnostic raw tarball.
		if u := match(func(n string) bool {
			return strings.Contains(n, "linux") && hasAnyArch(n, arches) && strings.HasSuffix(n, ".tar.gz")
		}); u != "" {
			return u
		}
		return match(func(n string) bool {
			return strings.Contains(n, "linux") && hasAnyArch(n, arches)
		})
	}
	return ""
}

// selectChecksums finds the platform's SHA256SUMS asset. Pure; unit-tested.
func selectChecksums(assets []ghAsset, goos string) string {
	suffix := "sha256sums-linux.txt"
	if goos == "darwin" {
		suffix = "sha256sums-macos.txt"
	}
	for _, a := range assets {
		n := strings.ToLower(a.Name)
		if strings.HasSuffix(n, suffix) || n == "sha256sums.txt" {
			return a.URL
		}
	}
	return ""
}

func hasAnyArch(name string, arches []string) bool {
	for _, a := range arches {
		if strings.Contains(name, a) {
			return true
		}
	}
	return false
}

// Check queries the release source and compares against currentVersion.
// A 404 (no releases published / private repo) is reported as "up to date"
// rather than an error, so the UI shows a benign message.
func Check(ctx context.Context, url, currentVersion string) (Info, error) {
	info := Info{CurrentVersion: currentVersion}

	ctx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return info, err
	}
	req.Header.Set("User-Agent", "helm-updater") // GitHub requires a UA
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return info, fmt.Errorf("contacting update server: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return info, nil // no releases published (or private) — treat as up to date
	case http.StatusForbidden, http.StatusTooManyRequests:
		return info, fmt.Errorf("update check rate-limited by GitHub (try again later)")
	default:
		return info, fmt.Errorf("update server returned %s", resp.Status)
	}

	body, err := readLimited(resp.Body, maxMetadataBytes)
	if err != nil {
		return info, err
	}
	rel, err := parseRelease(body)
	if err != nil {
		return info, err
	}

	info.LatestVersion = rel.TagName
	info.ReleaseURL = rel.HTMLURL
	info.ReleaseNotes = rel.Body
	info.UpdateAvailable = compareVersions(rel.TagName, currentVersion) > 0
	info.AssetURL = selectAsset(rel.Assets, runtime.GOOS, runtime.GOARCH)
	info.ChecksumsURL = selectChecksums(rel.Assets, runtime.GOOS)
	return info, nil
}

// parseChecksums maps filename → lowercase hex sha256 from a `sha256sum`-format
// file ("<hex>␠␠<name>"). Pure; unit-tested.
func parseChecksums(data string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || !isHex(fields[0]) {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*") // sha256sum binary marker
		out[filepath.Base(name)] = strings.ToLower(fields[0])
	}
	return out
}

// isHex reports whether s is a non-empty hexadecimal string.
func isHex(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

// Download streams assetURL to destDir and verifies it against the checksum for
// its filename found at checksumsURL. Fails closed: any mismatch (or missing
// checksum) deletes the file and returns an error. Returns the saved path.
func Download(ctx context.Context, assetURL, checksumsURL, destDir string) (string, error) {
	name := filepath.Base(assetURL)
	if name == "" || name == "." || name == "/" {
		return "", fmt.Errorf("cannot derive a filename from %q", assetURL)
	}
	// Fetch checksums first so we fail fast if verification is impossible.
	sums, err := fetch(ctx, checksumsURL)
	if err != nil {
		return "", fmt.Errorf("fetching checksums: %w", err)
	}
	want, ok := parseChecksums(string(sums))[name]
	if !ok {
		return "", fmt.Errorf("no published checksum for %s — refusing to install unverified", name)
	}

	dctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(dctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "helm-updater")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading %s: %s", name, resp.Status)
	}
	if resp.ContentLength > maxUpdateBytes {
		return "", fmt.Errorf("update %s exceeds the %d MiB size limit", name, maxUpdateBytes>>20)
	}

	f, err := os.CreateTemp(destDir, name+".part-*")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	h := sha256.New()
	written, err := io.CopyN(io.MultiWriter(f, h), resp.Body, maxUpdateBytes+1)
	if err != nil && !errors.Is(err, io.EOF) {
		f.Close()
		os.Remove(tmp)
		return "", err
	}
	if written > maxUpdateBytes {
		f.Close()
		os.Remove(tmp)
		return "", fmt.Errorf("update %s exceeds the %d MiB size limit", name, maxUpdateBytes>>20)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return "", err
	}

	got := hex.EncodeToString(h.Sum(nil))
	if got != want {
		os.Remove(tmp)
		return "", fmt.Errorf("checksum mismatch for %s: got %s, want %s", name, got, want)
	}
	dest, err := linkWithoutOverwrite(tmp, destDir, name)
	os.Remove(tmp)
	if err != nil {
		return "", err
	}
	return dest, nil
}

// linkWithoutOverwrite publishes a verified temporary file without replacing
// an existing download. Temp files live in destDir, so hard-link creation is
// atomic and remains on one filesystem on supported targets.
func linkWithoutOverwrite(tmp, destDir, name string) (string, error) {
	for i := 0; i < 1000; i++ {
		candidate := filepath.Join(destDir, name)
		if i > 0 {
			candidate = filepath.Join(destDir, fmt.Sprintf("%s.%d", name, i))
		}
		err := os.Link(tmp, candidate)
		if err == nil {
			return candidate, nil
		}
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return "", err
	}
	return "", fmt.Errorf("could not choose an unused destination for %s", name)
}

func readLimited(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("response exceeds %d-byte limit", limit)
	}
	return data, nil
}

func fetch(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "helm-updater")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return readLimited(resp.Body, maxMetadataBytes)
}
