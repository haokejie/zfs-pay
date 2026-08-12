package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/zoujunkun/zfs-pay/internal/app"
	"github.com/zoujunkun/zfs-pay/internal/enclosure"
	"github.com/zoujunkun/zfs-pay/internal/inventory"
)

const (
	statusSchemaVersion = 1
	defaultLocateTime   = 60 * time.Second
	maxLocateTime       = 24 * time.Hour
)

type Inventory interface {
	Discover(context.Context) (inventory.Snapshot, error)
}

type Enclosures interface {
	Map(context.Context, []app.Disk) ([]app.Disk, []app.Diagnostic)
	Plan(app.Bay, app.LEDAction) (enclosure.LocatePlan, error)
	Locate(context.Context, app.Bay, app.LEDAction) error
}

type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

type Runtime struct {
	Inventory  Inventory
	Enclosures Enclosures
	Build      BuildInfo
	Stdout     io.Writer
	Stderr     io.Writer
}

func (r Runtime) Run(ctx context.Context, args []string) int {
	stdout := r.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := r.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	if len(args) == 0 {
		printHelp(stdout)
		return 0
	}

	switch args[0] {
	case "help", "-h", "--help":
		if len(args) == 1 {
			printHelp(stdout)
			return 0
		}
		return printCommandHelp(args[1], stdout, stderr)
	case "version", "--version":
		fmt.Fprintf(stdout, "zfs-pay %s (commit %s, built %s)\n", nonEmpty(r.Build.Version, "dev"), nonEmpty(r.Build.Commit, "unknown"), nonEmpty(r.Build.Date, "unknown"))
		return 0
	case "status":
		return r.runStatus(ctx, args[1:], stdout, stderr)
	case "locate":
		return r.runLocate(ctx, args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "zfs-pay: unknown command %q\n", args[0])
		fmt.Fprintln(stderr, "Run 'zfs-pay help' for usage.")
		return 2
	}
}

func (r Runtime) runStatus(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	jsonOutput := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		case "-h", "--help":
			return printCommandHelp("status", stdout, stderr)
		default:
			fmt.Fprintf(stderr, "zfs-pay status: unknown option %q\n", arg)
			return 2
		}
	}
	if r.Inventory == nil || r.Enclosures == nil {
		return writeError(stderr, &app.Error{Code: app.ErrorInternal, Op: "status", Message: "runtime is not configured"})
	}
	snapshot, err := r.Inventory.Discover(ctx)
	if err != nil {
		return writeError(stderr, err)
	}
	disks, enclosureDiagnostics := r.Enclosures.Map(ctx, snapshot.Disks)
	snapshot.Disks = disks
	snapshot.Diagnostics = append(snapshot.Diagnostics, enclosureDiagnostics...)
	sortDisks(snapshot.Disks)
	if jsonOutput {
		payload := struct {
			SchemaVersion int              `json:"schema_version"`
			Disks         []app.Disk       `json:"disks"`
			Diagnostics   []app.Diagnostic `json:"diagnostics,omitempty"`
		}{SchemaVersion: statusSchemaVersion, Disks: snapshot.Disks, Diagnostics: snapshot.Diagnostics}
		encoder := json.NewEncoder(stdout)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(payload); err != nil {
			return writeError(stderr, &app.Error{Code: app.ErrorInternal, Op: "status", Message: "unable to encode JSON", Err: err})
		}
		return 0
	}
	printStatusTable(stdout, snapshot.Disks)
	printDiagnostics(stderr, snapshot)
	return 0
}

func (r Runtime) runLocate(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	options, err := parseLocateArgs(args)
	if err != nil {
		if errors.Is(err, errHelp) {
			return printCommandHelp("locate", stdout, stderr)
		}
		return writeError(stderr, err)
	}
	if r.Inventory == nil || r.Enclosures == nil {
		return writeError(stderr, &app.Error{Code: app.ErrorInternal, Op: "locate", Message: "runtime is not configured"})
	}
	snapshot, err := r.Inventory.Discover(ctx)
	if err != nil {
		return writeError(stderr, err)
	}
	disks, _ := r.Enclosures.Map(ctx, snapshot.Disks)
	disk, err := selectDisk(disks, options.target)
	if err != nil {
		return writeError(stderr, err)
	}
	if disk.Bay == nil || disk.Bay.Capability != app.LEDCapabilityLocate {
		return writeError(stderr, &app.Error{Code: app.ErrorUnsupported, Op: "locate", Message: "target has no supported locate LED backend"})
	}
	action := app.LEDActionOn
	if options.off {
		action = app.LEDActionOff
	}
	plan, err := r.Enclosures.Plan(*disk.Bay, action)
	if err != nil {
		return writeError(stderr, err)
	}
	if options.dryRun {
		fmt.Fprintf(stdout, "DRY RUN: %s\n", plan.String())
		if action == app.LEDActionOn {
			fmt.Fprintf(stdout, "DRY RUN: auto-off after %s\n", options.timeout)
		}
		return 0
	}
	if err := r.Enclosures.Locate(ctx, *disk.Bay, action); err != nil {
		return writeError(stderr, err)
	}
	fmt.Fprintf(stdout, "Locate LED %s: %s (%s)\n", action, disk.Bay.ID(), displayIdentity(*disk))
	if action == app.LEDActionOff {
		return 0
	}

	timer := time.NewTimer(options.timeout)
	defer timer.Stop()
	interrupted := false
	select {
	case <-timer.C:
	case <-ctx.Done():
		interrupted = true
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := r.Enclosures.Locate(cleanupCtx, *disk.Bay, app.LEDActionOff); err != nil {
		return writeError(stderr, err)
	}
	fmt.Fprintf(stdout, "Locate LED off: %s\n", disk.Bay.ID())
	if interrupted {
		return 130
	}
	return 0
}

type locateOptions struct {
	target  string
	off     bool
	dryRun  bool
	timeout time.Duration
}

var errHelp = errors.New("help requested")

func parseLocateArgs(args []string) (locateOptions, error) {
	options := locateOptions{timeout: defaultLocateTime}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			return options, errHelp
		case arg == "--off":
			options.off = true
		case arg == "--dry-run":
			options.dryRun = true
		case arg == "--timeout":
			if i+1 >= len(args) {
				return options, &app.Error{Code: app.ErrorMalformed, Op: "locate", Message: "--timeout requires a duration"}
			}
			i++
			duration, err := time.ParseDuration(args[i])
			if err != nil {
				return options, &app.Error{Code: app.ErrorMalformed, Op: "locate", Message: "invalid timeout", Err: err}
			}
			options.timeout = duration
		case strings.HasPrefix(arg, "--timeout="):
			duration, err := time.ParseDuration(strings.TrimPrefix(arg, "--timeout="))
			if err != nil {
				return options, &app.Error{Code: app.ErrorMalformed, Op: "locate", Message: "invalid timeout", Err: err}
			}
			options.timeout = duration
		case strings.HasPrefix(arg, "-"):
			return options, &app.Error{Code: app.ErrorMalformed, Op: "locate", Message: fmt.Sprintf("unknown option %q", arg)}
		case options.target == "":
			options.target = arg
		default:
			return options, &app.Error{Code: app.ErrorMalformed, Op: "locate", Message: "multiple targets provided"}
		}
	}
	if options.target == "" {
		return options, &app.Error{Code: app.ErrorMalformed, Op: "locate", Message: "TARGET is required"}
	}
	if !options.off && (options.timeout <= 0 || options.timeout > maxLocateTime) {
		return options, &app.Error{Code: app.ErrorMalformed, Op: "locate", Message: "timeout must be greater than 0 and no more than 24h"}
	}
	return options, nil
}

func selectDisk(disks []app.Disk, target string) (*app.Disk, error) {
	normalized := enclosure.NormalizeIdentity(target)
	var matches []int
	for i := range disks {
		bayID := ""
		if disks[i].Bay != nil {
			bayID = disks[i].Bay.ID()
		}
		if target == disks[i].DevicePath || target == disks[i].ParentDevice || target == disks[i].VdevGUID || target == bayID ||
			(normalized != "" && normalized == enclosure.NormalizeIdentity(disks[i].Serial)) {
			matches = append(matches, i)
		}
	}
	switch len(matches) {
	case 0:
		return nil, &app.Error{Code: app.ErrorNotFound, Op: "locate", Message: "target disk not found"}
	case 1:
		return &disks[matches[0]], nil
	default:
		return nil, &app.Error{Code: app.ErrorAmbiguous, Op: "locate", Message: "target matches multiple disks; use a GUID or full bay ID"}
	}
}

func printStatusTable(w io.Writer, disks []app.Disk) {
	if len(disks) == 0 {
		fmt.Fprintln(w, "No ZFS disks found.")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "POOL\tSTATE\tDEVICE\tSERIAL\tBAY\tBACKEND\tLED")
	for _, disk := range disks {
		bay, backend, led := "-", "-", "-"
		if disk.Bay != nil {
			bay = nonEmpty(disk.Bay.ID(), "-")
			backend = nonEmpty(disk.Bay.Backend, "-")
			led = string(disk.Bay.LEDState)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			nonEmpty(disk.Pool, "-"), nonEmpty(string(disk.State), "-"),
			nonEmpty(disk.ParentDevice, nonEmpty(disk.DevicePath, "-")), nonEmpty(disk.Serial, "-"),
			bay, backend, nonEmpty(led, "-"))
	}
	_ = tw.Flush()
}

func printDiagnostics(w io.Writer, snapshot inventory.Snapshot) {
	for _, diagnostic := range snapshot.Diagnostics {
		fmt.Fprintf(w, "warning[%s] %s: %s\n", diagnostic.Code, nonEmpty(diagnostic.Source, "status"), diagnostic.Message)
	}
	for _, disk := range snapshot.Disks {
		for _, diagnostic := range disk.Diagnostics {
			fmt.Fprintf(w, "warning[%s] %s %s: %s\n", diagnostic.Code, nonEmpty(disk.VdevGUID, disk.DevicePath), nonEmpty(diagnostic.Source, "disk"), diagnostic.Message)
		}
	}
}

func sortDisks(disks []app.Disk) {
	sort.SliceStable(disks, func(i, j int) bool {
		leftBay, rightBay := "", ""
		if disks[i].Bay != nil {
			leftBay = disks[i].Bay.ID()
		}
		if disks[j].Bay != nil {
			rightBay = disks[j].Bay.ID()
		}
		left := disks[i].Pool + "\x00" + leftBay + "\x00" + disks[i].ParentDevice + "\x00" + disks[i].VdevGUID
		right := disks[j].Pool + "\x00" + rightBay + "\x00" + disks[j].ParentDevice + "\x00" + disks[j].VdevGUID
		return left < right
	})
}

func displayIdentity(disk app.Disk) string {
	return nonEmpty(disk.Serial, nonEmpty(disk.ParentDevice, nonEmpty(disk.DevicePath, disk.VdevGUID)))
}

func writeError(w io.Writer, err error) int {
	fmt.Fprintf(w, "zfs-pay: %s\n", err)
	var typed *app.Error
	if errors.As(err, &typed) {
		if typed.Code == app.ErrorMalformed {
			return 2
		}
		return app.ExitCode(typed.Code)
	}
	return 1
}

func nonEmpty(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage: zfs-pay COMMAND [OPTIONS]

Commands:
  status          Show ZFS disks and physical bays
  locate TARGET   Control one drive locate LED
  help            Show this help

Options:
  --version       Show version`)
}

func printCommandHelp(command string, stdout, stderr io.Writer) int {
	switch command {
	case "status":
		fmt.Fprintln(stdout, `Usage: zfs-pay status [--json]

Show ZFS leaf vdevs, Linux disk identity, physical bay, backend, and LED state.`)
		return 0
	case "locate":
		fmt.Fprintln(stdout, `Usage: zfs-pay locate TARGET [--off] [--dry-run] [--timeout DURATION]

TARGET is an exact device path, serial, ZFS GUID, or controller/enclosure/slot.
Locate LEDs turn off automatically after 60s unless --off is used.`)
		return 0
	case "help":
		fmt.Fprintln(stdout, "Usage: zfs-pay help [COMMAND]")
		return 0
	default:
		fmt.Fprintf(stderr, "zfs-pay: no help for %q\n", command)
		return 2
	}
}
