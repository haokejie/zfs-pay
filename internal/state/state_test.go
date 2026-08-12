package state

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/zoujunkun/zfs-pay/internal/app"
)

func TestStoreRoundTripAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store := Store{Path: path}
	_, err := store.Update(context.Background(), func(data *Data) error {
		data.Mappings["101"] = Mapping{GUID: "101", Serial: "SERIAL-101", Bay: app.Bay{Backend: "fake", Controller: "c0", Enclosure: "e1", Slot: "s2"}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if data.SchemaVersion != SchemaVersion || data.Mappings["101"].Bay.ID() != "c0/e1/s2" {
		t.Fatalf("unexpected data: %#v", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
}

func TestStoreRecoversCorruptState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := Store{Path: path, Now: func() time.Time { return time.Unix(10, 20) }}
	recovered, err := store.Update(context.Background(), func(data *Data) error {
		data.LastEvents["101"] = "event-1"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !recovered {
		t.Fatal("expected corrupt state recovery")
	}
	if _, err := os.Stat(path + ".corrupt-10000000020"); err != nil {
		t.Fatalf("corrupt backup missing: %v", err)
	}
}

func TestStoreSerializesConcurrentUpdates(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "state.json")}
	var group sync.WaitGroup
	for i := 0; i < 20; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := store.Update(context.Background(), func(data *Data) error {
				mapping := data.Mappings["101"]
				mapping.GUID = "101"
				mapping.Pool += "x"
				data.Mappings["101"] = mapping
				return nil
			}); err != nil {
				t.Errorf("update: %v", err)
			}
		}()
	}
	group.Wait()
	data, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Mappings["101"].Pool) != 20 {
		t.Fatalf("lost update: %q", data.Mappings["101"].Pool)
	}
}
