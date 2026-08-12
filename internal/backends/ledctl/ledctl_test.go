package ledctl

import (
	"reflect"
	"testing"

	"github.com/zoujunkun/zfs-pay/internal/app"
	"github.com/zoujunkun/zfs-pay/internal/system"
)

func TestCommandForLocateAllowlist(t *testing.T) {
	t.Parallel()

	bay := app.Bay{Controller: "linux", Enclosure: "block", Slot: "sda"}
	command, err := CommandForLocate("/usr/sbin/ledctl", bay, app.LEDActionOff)
	if err != nil {
		t.Fatal(err)
	}
	want := system.Command{Name: "/usr/sbin/ledctl", Args: []string{"locate_off=/dev/sda"}}
	if !reflect.DeepEqual(command, want) {
		t.Fatalf("command = %#v, want %#v", command, want)
	}
	if _, err := CommandForLocate("/usr/sbin/ledctl", app.Bay{Controller: "linux", Enclosure: "block", Slot: "sda other"}, app.LEDActionOn); err == nil {
		t.Fatal("unsafe target was accepted")
	}
	if _, err := CommandForLocate("/usr/sbin/ledctl", app.Bay{Controller: "linux", Enclosure: "block", Slot: ".."}, app.LEDActionOn); err == nil {
		t.Fatal("path traversal target was accepted")
	}
}

func TestBackendDisabledByDefault(t *testing.T) {
	t.Parallel()

	if _, err := (Backend{}).List(nil, nil); err == nil {
		t.Fatal("disabled backend returned no error")
	}
}
