package automation

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zoujunkun/zfs-pay/internal/app"
)

func TestEventFromLookupFixture(t *testing.T) {
	file, err := os.Open(filepath.Join("..", "..", "testdata", "zed", "faulted-event.env"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if ok {
			values[key] = value
		}
	}
	event, err := EventFromLookup(func(key string) string { return values[key] })
	if err != nil {
		t.Fatal(err)
	}
	if event.GUID != "101" || event.State != app.HealthFaulted || event.VdevType != "disk" {
		t.Fatalf("unexpected event: %#v", event)
	}
}

func TestEventValidation(t *testing.T) {
	tests := []struct {
		name    string
		event   Event
		wantErr bool
	}{
		{name: "disk", event: Event{Pool: "tank", GUID: "101", State: app.HealthOnline, VdevType: "disk"}},
		{name: "non disk ignored", event: Event{VdevType: "mirror"}},
		{name: "missing guid", event: Event{Pool: "tank", State: app.HealthFaulted, VdevType: "disk"}, wantErr: true},
		{name: "non decimal guid", event: Event{Pool: "tank", GUID: "disk-1", State: app.HealthFaulted, VdevType: "disk"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.event.Validate()
			if (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}
