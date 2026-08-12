package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/zoujunkun/zfs-pay/internal/backends/ledctl"
	"github.com/zoujunkun/zfs-pay/internal/backends/ses"
	"github.com/zoujunkun/zfs-pay/internal/backends/storcli"
	"github.com/zoujunkun/zfs-pay/internal/cli"
	"github.com/zoujunkun/zfs-pay/internal/enclosure"
	"github.com/zoujunkun/zfs-pay/internal/inventory"
	"github.com/zoujunkun/zfs-pay/internal/system"
	"github.com/zoujunkun/zfs-pay/internal/zfs"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:]))
}

func run(ctx context.Context, args []string) int {
	runner := system.ExecRunner{}
	inventoryService := &inventory.Discoverer{
		ZFS:      zfs.Provider{Runner: runner},
		Identity: system.BlockProvider{Runner: runner},
	}
	enclosureService := &enclosure.Service{Backends: []enclosure.Backend{
		storcli.Backend{Runner: runner},
		ses.Backend{},
		ledctl.Backend{Runner: runner, Enabled: os.Getenv("ZFS_PAY_ENABLE_LEDCTL") == "1"},
	}}
	runtime := cli.Runtime{
		Inventory:  inventoryService,
		Enclosures: enclosureService,
		Build:      cli.BuildInfo{Version: version, Commit: commit, Date: buildDate},
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
	}
	return runtime.Run(ctx, args)
}
