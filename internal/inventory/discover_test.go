package inventory

import (
	"context"
	"errors"
	"testing"

	"github.com/zoujunkun/zfs-pay/internal/app"
	"github.com/zoujunkun/zfs-pay/internal/system"
	"github.com/zoujunkun/zfs-pay/internal/zfs"
)

type stubZFS struct {
	leaves []zfs.Leaf
	err    error
}

func (s stubZFS) Discover(context.Context) ([]zfs.Leaf, error) { return s.leaves, s.err }

type stubIdentity struct {
	identities  map[string]system.Identity
	diagnostics map[string][]app.Diagnostic
	err         error
}

func (s stubIdentity) Resolve(context.Context, []string) (map[string]system.Identity, map[string][]app.Diagnostic, error) {
	return s.identities, s.diagnostics, s.err
}

func TestDiscoverJoinsIdentityAndPreservesMissingDisk(t *testing.T) {
	t.Parallel()

	discoverer := Discoverer{
		ZFS: stubZFS{leaves: []zfs.Leaf{
			{Pool: "tank", GUID: "1", Name: "disk-a", Path: "/dev/by-id/a", Role: "data", State: app.HealthOnline},
			{Pool: "tank", GUID: "2", Name: "2", Role: "data", State: app.HealthUnavail},
		}},
		Identity: stubIdentity{identities: map[string]system.Identity{
			"/dev/by-id/a": {Device: "/dev/sda1", ParentDevice: "/dev/sda", Serial: "SERIAL-A", WWN: "5000", Model: "Disk"},
		}},
	}
	snapshot, err := discoverer.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(snapshot.Disks), 2; got != want {
		t.Fatalf("len(disks) = %d, want %d", got, want)
	}
	if snapshot.Disks[0].Serial != "SERIAL-A" || snapshot.Disks[0].ParentDevice != "/dev/sda" {
		t.Fatalf("joined disk = %#v", snapshot.Disks[0])
	}
	if snapshot.Disks[1].VdevGUID != "2" || snapshot.Disks[1].State != app.HealthUnavail {
		t.Fatalf("missing disk = %#v", snapshot.Disks[1])
	}
}

func TestDiscoverTreatsIdentityFailureAsPartial(t *testing.T) {
	t.Parallel()

	discoverer := Discoverer{
		ZFS:      stubZFS{leaves: []zfs.Leaf{{Pool: "tank", GUID: "1", Path: "/dev/sda", State: app.HealthOnline}}},
		Identity: stubIdentity{err: &app.Error{Code: app.ErrorUnsupported, Op: "lsblk", Message: "command not found"}},
	}
	snapshot, err := discoverer.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Disks) != 1 || len(snapshot.Diagnostics) != 1 || snapshot.Diagnostics[0].Code != app.ErrorUnsupported {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestDiscoverReturnsZFSError(t *testing.T) {
	t.Parallel()

	want := errors.New("zfs failed")
	_, err := (Discoverer{ZFS: stubZFS{err: want}}).Discover(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("Discover() error = %v, want %v", err, want)
	}
}
