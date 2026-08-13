package polkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"helm/internal/service"
)

func TestSystemUnits(t *testing.T) {
	units := SystemUnits([]service.Service{
		{ID: "ollama", Kind: service.KindSystemctl, Unit: "ollama"},        // bare -> .service
		{ID: "custom", Kind: service.KindSystemctl, Unit: "myllm.service"}, // suffixed -> untouched
		{ID: "dupe", Kind: service.KindSystemctl, Unit: "ollama.service"},  // dedupes with the first
		{ID: "webui", Kind: service.KindDocker, Container: "open-webui"},   // non-systemctl kinds excluded
		{ID: "hermes", Kind: service.KindProcess, Port: 9119},              //
		{ID: "probe", Kind: service.KindPort, Port: 7860},                  //
		{ID: "broken", Kind: service.KindSystemctl, Unit: ""},              // no unit -> skipped
	})
	want := []string{"ollama.service", "myllm.service"}
	if len(units) != len(want) {
		t.Fatalf("SystemUnits = %v, want %v", units, want)
	}
	for i := range want {
		if units[i] != want[i] {
			t.Errorf("units[%d] = %q, want %q", i, units[i], want[i])
		}
	}
}

func TestGenerateRule(t *testing.T) {
	rule := GenerateRule([]string{"ollama.service", "myllm.service"})

	// Security constraints: exactly one action.id, admin-group-gated, only the
	// listed units, only start/stop/restart.
	if n := strings.Count(rule, "action.id =="); n != 1 {
		t.Errorf("rule checks %d action ids, want exactly 1", n)
	}
	for _, must := range []string{
		`action.id == "org.freedesktop.systemd1.manage-units"`,
		`subject.isInGroup("wheel")`,
		`subject.isInGroup("sudo")`,
		`"ollama.service"`,
		`"myllm.service"`,
		`["start", "stop", "restart"]`,
		`polkit.Result.YES`,
	} {
		if !strings.Contains(rule, must) {
			t.Errorf("rule is missing %q:\n%s", must, rule)
		}
	}
	// YES must be reachable only behind the unit allowlist check.
	if !strings.Contains(rule, "allowedUnits.indexOf(unit) >= 0") {
		t.Error("rule grants YES without checking the unit allowlist")
	}
}

func TestGenerateRuleEmptyUnitsGrantsNothing(t *testing.T) {
	if rule := GenerateRule(nil); !strings.Contains(rule, "var allowedUnits = [];") {
		t.Errorf("empty unit list must produce an empty allowlist:\n%s", rule)
	}
}

func TestInstallWritesRuleFile(t *testing.T) {
	// Path is parameterized precisely so this test never touches /etc.
	path := filepath.Join(t.TempDir(), "polkit-1", "rules.d", "99-helm.rules")
	rule := GenerateRule([]string{"ollama.service"})

	if err := Install(path, rule); err != nil {
		t.Fatalf("Install: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading installed rule: %v", err)
	}
	if string(raw) != rule {
		t.Error("installed content differs from the generated rule")
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o644 {
		t.Errorf("rule file mode = %o, want 0644 (world-readable, root-writable)", fi.Mode().Perm())
	}
}
