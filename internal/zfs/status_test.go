package zfs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zoujunkun/zfs-pay/internal/app"
)

func TestParseStatusWithUPath(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "zfs", "status-upath.txt"))
	if err != nil {
		t.Fatal(err)
	}
	leaves, err := ParseStatus(data, true)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(leaves), 6; got != want {
		t.Fatalf("len(leaves) = %d, want %d: %#v", got, want, leaves)
	}

	tests := []struct {
		index int
		pool  string
		guid  string
		role  string
		state app.Health
		path  string
	}{
		{0, "tank", "1000000000000001", "data", app.HealthOnline, "/dev/disk/by-id/scsi-DISK_A-part1"},
		{1, "tank", "1000000000000002", "data", app.HealthOnline, "/dev/disk/by-id/scsi-DISK_B-part1"},
		{2, "tank", "2000000000000001", "log", app.HealthOnline, "/dev/disk/by-id/scsi-LOG_A-part1"},
		{3, "tank", "3000000000000001", "spare", app.HealthAvail, "/dev/disk/by-id/scsi-SPARE_A"},
		{4, "archive", "4000000000000001", "data", app.HealthOnline, "/dev/sde"},
		{5, "archive", "4000000000000002", "data", app.HealthUnavail, ""},
	}
	for _, tt := range tests {
		leaf := leaves[tt.index]
		if leaf.Pool != tt.pool || leaf.GUID != tt.guid || leaf.Role != tt.role || leaf.State != tt.state || leaf.Path != tt.path {
			t.Errorf("leaf[%d] = %#v", tt.index, leaf)
		}
	}
	if leaves[1].Errors.Checksum != 2 {
		t.Fatalf("checksum errors = %d, want 2", leaves[1].Errors.Checksum)
	}
}

func TestMergeSnapshotsRejectsTopologyChange(t *testing.T) {
	t.Parallel()

	guid := []Leaf{{Pool: "tank", Name: "123", Role: "data", State: app.HealthOnline, Depth: 10}}
	path := []Leaf{{Pool: "tank", Name: "/dev/sda", Path: "/dev/sda", Role: "data", State: app.HealthFaulted, Depth: 10}}
	if _, err := MergeSnapshots(guid, path); err == nil {
		t.Fatal("MergeSnapshots() error = nil, want topology error")
	}
}

func TestParseStatusRejectsBadCounters(t *testing.T) {
	t.Parallel()

	data := []byte("  pool: tank\nconfig:\n\n NAME STATE READ WRITE CKSUM\n tank ONLINE 0 0 0\n   /dev/sda ONLINE bad 0 0\nerrors: none\n")
	if _, err := ParseStatus(data, false); err == nil {
		t.Fatal("ParseStatus() error = nil, want malformed output")
	}
}
