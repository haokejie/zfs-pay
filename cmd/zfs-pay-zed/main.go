// zfs-pay-zed is an internal entry point installed for ZED and systemd. It is
// intentionally separate from the small, user-facing zfs-pay command set.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zoujunkun/zfs-pay/internal/app"
	"github.com/zoujunkun/zfs-pay/internal/automation"
	"github.com/zoujunkun/zfs-pay/internal/backends/ledctl"
	"github.com/zoujunkun/zfs-pay/internal/backends/ses"
	"github.com/zoujunkun/zfs-pay/internal/backends/storcli"
	"github.com/zoujunkun/zfs-pay/internal/enclosure"
	"github.com/zoujunkun/zfs-pay/internal/inventory"
	"github.com/zoujunkun/zfs-pay/internal/state"
	"github.com/zoujunkun/zfs-pay/internal/system"
	"github.com/zoujunkun/zfs-pay/internal/zfs"
)

const defaultStatePath = "/var/lib/zfs-pay/state.json"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, lookup func(string) string, stdout, stderr io.Writer) int {
	mode, dryRun, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return app.ExitCode(app.ErrorMalformed)
	}
	runner := system.ExecRunner{}
	inventoryService := &inventory.Discoverer{
		ZFS:      zfs.Provider{Runner: runner},
		Identity: system.BlockProvider{Runner: runner},
	}
	enclosureService := &enclosure.Service{Backends: []enclosure.Backend{
		storcli.Backend{Runner: runner},
		ses.Backend{},
		ledctl.Backend{Runner: runner, Enabled: lookup("ZFS_PAY_ENABLE_LEDCTL") == "1"},
	}}
	statePath := lookup("ZFS_PAY_STATE_PATH")
	if statePath == "" {
		statePath = defaultStatePath
	}
	logger := slog.New(slog.NewJSONHandler(stderr, nil))
	engine := automation.Engine{
		Inventory: inventoryService, Enclosures: enclosureService,
		Store: state.Store{Path: statePath}, Timeout: commandTimeout(lookup), Logger: logger,
	}
	var result automation.Result
	switch mode {
	case "event":
		event, parseErr := automation.EventFromLookup(lookup)
		if parseErr != nil {
			err = parseErr
			break
		}
		result, err = engine.Handle(ctx, event, dryRun)
	case "reconcile":
		result, err = engine.Reconcile(ctx, dryRun)
	case "cleanup":
		result, err = engine.Cleanup(ctx, dryRun)
	}
	if err != nil {
		logger.Error("automation failed", "mode", mode, "error", err)
		var typed *app.Error
		if errors.As(err, &typed) {
			return app.ExitCode(typed.Code)
		}
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(result); err != nil {
		logger.Error("unable to encode result", "error", err)
		return 1
	}
	return 0
}

func parseArgs(args []string) (string, bool, error) {
	if len(args) < 1 || len(args) > 2 || (args[0] != "event" && args[0] != "reconcile" && args[0] != "cleanup") {
		return "", false, fmt.Errorf("usage: zfs-pay-zed <event|reconcile|cleanup> [--dry-run]")
	}
	if len(args) == 2 && args[1] != "--dry-run" {
		return "", false, fmt.Errorf("usage: zfs-pay-zed <event|reconcile|cleanup> [--dry-run]")
	}
	return args[0], len(args) == 2, nil
}

func commandTimeout(lookup func(string) string) time.Duration {
	value := lookup("ZFS_PAY_COMMAND_TIMEOUT")
	if value == "" {
		return 20 * time.Second
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 || duration > 5*time.Minute {
		return 20 * time.Second
	}
	return duration
}
