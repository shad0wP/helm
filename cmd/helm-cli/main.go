// Command helm-cli is the terminal face of Helm — the exact same product as
// the tray app: same service engine, same defaults (Ollama, Open WebUI,
// SearXNG, Hermes Agent, OpenClaw), same ~/.config/helm/services.json, same
// auto-discovery. It exists so the stack can be toggled from scripts, ssh
// sessions, and keybindings, with a VRAM delta report when nvidia-smi is
// available.
package main

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"helm/internal/gpu"
	"helm/internal/polkit"
	"helm/internal/service"
)

// version is injected at build time via -ldflags "-X main.version=<v>".
var version = "dev"

// settleDelay is the post-toggle flush window before the VRAM re-read, giving
// processes time to actually release GPU memory.
const settleDelay = 2 * time.Second

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	cmd := "toggle"
	if len(args) > 0 {
		cmd = args[0]
	}
	switch cmd {
	case "toggle":
		return runToggle()
	case "on":
		return runBulk(true)
	case "off":
		return runBulk(false)
	case "status":
		return runStatus()
	case "free-vram":
		return runFreeVRAM()
	case "setup-polkit":
		return runSetupPolkit()
	case "version", "-v", "--version":
		fmt.Println(version)
		return 0
	case "help", "-h", "--help":
		usage(os.Stdout)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "helm-cli: unknown command %q\n\n", cmd)
		usage(os.Stderr)
		return 2
	}
}

func usage(w *os.File) {
	fmt.Fprint(w, `helm-cli — toggle your local AI stack from the terminal (same engine as the Helm tray app)

Usage:
  helm-cli               Toggle: stop everything if anything runs, start everything if not
  helm-cli on            Start every controllable, stopped service
  helm-cli off           Stop every controllable, running service
  helm-cli status        Per-service state, aggregate state, and VRAM in use
  helm-cli free-vram     Evict all loaded Ollama models (frees VRAM, daemon stays up)
  helm-cli setup-polkit  Install the polkit rule for passwordless systemd control (root, Linux)
  helm-cli version       Print the version

Configuration: ~/.config/helm/services.json — shared with the tray app.
`)
}

func runToggle() int {
	m := service.NewServiceManager()
	// Mirror the tray semantics: gray (nothing running) means start the
	// stack; green/amber means stop it.
	start := service.AggregateState(m.GetServices()) == "none"
	return bulkWith(m, start)
}

func runBulk(start bool) int {
	return bulkWith(service.NewServiceManager(), start)
}

func bulkWith(m *service.ServiceManager, start bool) int {
	g := gpu.New()
	before, haveGPU := g.UsedMB()

	verb := "Stopping"
	if start {
		verb = "Starting"
	}
	fmt.Printf("%s services...\n", verb)
	var err error
	if start {
		err = m.StartAll()
	} else {
		err = m.StopAll()
	}
	for _, s := range m.GetServices() {
		state := "stopped"
		if s.Running {
			state = "running"
		}
		fmt.Printf("  %-24s %s\n", s.Name, state)
	}

	if haveGPU {
		time.Sleep(settleDelay)
		if after, ok := g.UsedMB(); ok {
			fmt.Println(gpu.FormatDelta(before, after, !start))
		}
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "helm-cli: %v\n", err)
		return 1
	}
	return 0
}

func runStatus() int {
	m := service.NewServiceManager()
	svcs := m.GetServices()
	fmt.Printf("Aggregate: %s\n", service.AggregateState(svcs))
	for _, s := range svcs {
		state := "stopped"
		if s.Running {
			state = "running"
		}
		fmt.Printf("  %-24s %-10s %s\n", s.Name, s.Kind, state)
	}
	if used, ok := gpu.New().UsedMB(); ok {
		fmt.Printf("VRAM in use: %d MB\n", used)
	}
	return 0
}

func runFreeVRAM() int {
	n, err := service.NewServiceManager().FreeVRAM()
	if err != nil {
		fmt.Fprintf(os.Stderr, "helm-cli: %v\n", err)
		return 1
	}
	fmt.Printf("Evicted %d model(s) from VRAM.\n", n)
	return 0
}

func runSetupPolkit() int {
	if runtime.GOOS != "linux" {
		fmt.Fprintln(os.Stderr, "helm-cli: setup-polkit is Linux-only (polkit governs systemd units)")
		return 1
	}
	// The snapshot already merges defaults with ~/.config/helm/services.json,
	// so the rule covers exactly the units Helm actually manages.
	units := polkit.SystemUnits(service.NewServiceManager().GetServices())
	if len(units) == 0 {
		fmt.Println("No systemd system services configured — nothing for polkit to allow.")
		return 0
	}
	rule := polkit.GenerateRule(units)
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "helm-cli: setup-polkit must run as root: sudo helm-cli setup-polkit")
		fmt.Fprintln(os.Stderr, "The rule that would be installed to "+polkit.RulePath+":")
		fmt.Print(rule)
		return 1
	}
	if err := polkit.Install(polkit.RulePath, rule); err != nil {
		fmt.Fprintf(os.Stderr, "helm-cli: installing polkit rule: %v\n", err)
		return 1
	}
	fmt.Printf("Installed %s covering: %v\n", polkit.RulePath, units)
	return 0
}
