package system

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type fakeRunner struct {
	results map[string]Result
}

func (f fakeRunner) Run(_ context.Context, command Command) (Result, error) {
	key := command.Name
	if command.Name == "udevadm" && len(command.Args) > 0 {
		key += ":" + command.Args[len(command.Args)-1]
	}
	return f.results[key], nil
}

func TestBlockProviderResolvesPartitionAndUdevFallback(t *testing.T) {
	t.Parallel()

	lsblk, err := os.ReadFile(filepath.Join("..", "..", "testdata", "zfs", "lsblk.json"))
	if err != nil {
		t.Fatal(err)
	}
	provider := BlockProvider{
		Runner: fakeRunner{results: map[string]Result{
			"lsblk":            {Stdout: lsblk},
			"udevadm:/dev/sdb": {Stdout: []byte("ID_SCSI_SERIAL=disk-b-serial\nID_WWN=0x5000000000000002\nID_MODEL=Fallback_Model\n")},
		}},
		ResolvePath: func(value string) (string, error) {
			mapping := map[string]string{
				"/dev/disk/by-id/scsi-DISK_A-part1": "/dev/sda1",
				"/dev/disk/by-id/scsi-DISK_B":       "/dev/sdb",
			}
			return mapping[value], nil
		},
	}
	paths := []string{"/dev/disk/by-id/scsi-DISK_A-part1", "/dev/disk/by-id/scsi-DISK_B"}
	identities, diagnostics, err := provider.Resolve(context.Background(), paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}

	wantA := Identity{Device: "/dev/sda1", ParentDevice: "/dev/sda", Serial: "DISK-A-SERIAL", WWN: "5000000000000001", Model: "Example SAS Disk"}
	if got := identities[paths[0]]; !reflect.DeepEqual(got, wantA) {
		t.Fatalf("identity A = %#v, want %#v", got, wantA)
	}
	wantB := Identity{Device: "/dev/sdb", ParentDevice: "/dev/sdb", Serial: "DISK-B-SERIAL", WWN: "5000000000000002", Model: "Fallback_Model"}
	if got := identities[paths[1]]; !reflect.DeepEqual(got, wantB) {
		t.Fatalf("identity B = %#v, want %#v", got, wantB)
	}
}

func TestParseProperties(t *testing.T) {
	t.Parallel()

	got := parseProperties([]byte("A=1\nBROKEN\nB=two=parts\n"))
	want := map[string]string{"A": "1", "B": "two=parts"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseProperties() = %#v, want %#v", got, want)
	}
}
