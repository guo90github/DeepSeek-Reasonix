package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"

	"reasonix/internal/agentd"
	"reasonix/internal/config"
	"reasonix/internal/i18n"
)

// runAgents manages the serve instances that give each workspace its own
// addressable session endpoint. A managed instance dies with its manager (the
// child sits in a kill-on-close job), so `up` supervises in the foreground
// rather than starting and leaving: the manager is the lifecycle owner.
func runAgents(args []string) int {
	manager, ok := newAgentManager()
	if !ok {
		return 1
	}
	if len(args) == 0 {
		agentsUsage()
		return 2
	}
	switch args[0] {
	case "ls", "list":
		return agentsList(manager)
	case "up":
		return agentsUp(manager, args[1:])
	case "down":
		return agentsDown(manager, args[1:])
	default:
		agentsUsage()
		return 2
	}
}

func agentsUsage() {
	fmt.Fprintln(os.Stderr, "usage: reasonix agents <ls|up|down>")
	fmt.Fprintln(os.Stderr, "  ls                     probe every recorded instance")
	fmt.Fprintln(os.Stderr, "  up <root> [--name N] [--session PATH] [--addr A]")
	fmt.Fprintln(os.Stderr, "  down <name>            stop an instance this process manages")
}

func newAgentManager() (*agentd.Manager, bool) {
	home := strings.TrimSpace(config.ReasonixHomeDir())
	if home == "" {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, "no Reasonix home is resolvable")
		return nil, false
	}
	return agentd.NewManager(home), true
}

func agentsList(manager *agentd.Manager) int {
	records, err := manager.Status(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
		return 1
	}
	if len(records) == 0 {
		fmt.Println("no managed agents")
		return 0
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATE\tADDR\tPID\tROOT")
	for _, rec := range records {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n", rec.Name, rec.State, rec.Addr, rec.PID, rec.Root)
	}
	_ = w.Flush()
	return 0
}

func agentsUp(manager *agentd.Manager, args []string) int {
	spec, ok := parseAgentSpec(args)
	if !ok {
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	rec, err := manager.Up(ctx, spec)
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
		return 1
	}
	fmt.Printf("agent %s serves %s on %s (token %s)\n", rec.Name, rec.Root, rec.Addr, rec.TokenFile)
	fmt.Println("supervising; press Ctrl-C to stop it")
	if err := manager.Watch(ctx); err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
		return 1
	}
	manager.StopAll()
	return 0
}

func agentsDown(manager *agentd.Manager, args []string) int {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		agentsUsage()
		return 2
	}
	if err := manager.Down(args[0]); err != nil {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
		return 1
	}
	fmt.Printf("agent %s stopped\n", args[0])
	return 0
}

// parseAgentSpec reads `up`'s arguments. The name defaults to the workspace's
// directory name, which is what a room's members and sessions already key on.
func parseAgentSpec(args []string) (agentd.Spec, bool) {
	var spec agentd.Spec
	for i := 0; i < len(args); i++ {
		value := func() (string, bool) {
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, args[i], "needs a value")
				return "", false
			}
			i++
			return args[i], true
		}
		switch args[i] {
		case "--name", "-name":
			v, ok := value()
			if !ok {
				return spec, false
			}
			spec.Name = v
		case "--session", "-session":
			v, ok := value()
			if !ok {
				return spec, false
			}
			spec.SessionPath = v
		case "--addr", "-addr":
			v, ok := value()
			if !ok {
				return spec, false
			}
			spec.Addr = v
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, "unknown flag", args[i])
				return spec, false
			}
			if spec.Root == "" {
				spec.Root = args[i]
			}
		}
	}
	if spec.Root == "" {
		agentsUsage()
		return spec, false
	}
	if spec.Name == "" {
		spec.Name = filepath.Base(filepath.Clean(spec.Root))
	}
	return spec, true
}
