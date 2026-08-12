package ses

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/zoujunkun/zfs-pay/internal/app"
)

func TestBackendListsAndControlsMappedSlot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	slotDir := filepath.Join(root, "class", "enclosure", "0:0:41:0", "Array Device 02")
	if err := os.MkdirAll(slotDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(slotDir, "slot"), []byte("2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(slotDir, "locate"), []byte("0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deviceDir := filepath.Join(root, "class", "block", "sda", "device")
	if err := os.MkdirAll(deviceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(slotDir, filepath.Join(deviceDir, "enclosure_device:0")); err != nil {
		t.Fatal(err)
	}

	backend := Backend{Root: root}
	disks := []app.Disk{{ParentDevice: "/dev/sda", Serial: "SERIAL-A", WWN: "5000", Model: "Disk"}}
	slots, err := backend.List(context.Background(), disks)
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 1 || slots[0].Bay.Enclosure != "0:0:41:0" || slots[0].Bay.Slot != "2" {
		t.Fatalf("slots = %#v", slots)
	}
	if err := backend.Locate(context.Background(), slots[0].Bay, app.LEDActionOn); err != nil {
		t.Fatal(err)
	}
	value, err := os.ReadFile(filepath.Join(slotDir, "locate"))
	if err != nil {
		t.Fatal(err)
	}
	if string(value) != "1" {
		t.Fatalf("locate value = %q, want 1", value)
	}
}

func TestBackendRejectsPathTraversal(t *testing.T) {
	t.Parallel()

	err := (Backend{Root: t.TempDir()}).Locate(context.Background(), app.Bay{Controller: "ses", Enclosure: "..", Slot: "2"}, app.LEDActionOn)
	if err == nil {
		t.Fatal("path traversal target was accepted")
	}
}
