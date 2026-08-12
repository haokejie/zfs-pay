package app

import "testing"

func TestBayID(t *testing.T) {
	t.Parallel()

	bay := Bay{Controller: "c0", Enclosure: "e41", Slot: "s2"}
	if got, want := bay.ID(), "c0/e41/s2"; got != want {
		t.Fatalf("Bay.ID() = %q, want %q", got, want)
	}

	if got := (Bay{Controller: "c0"}).ID(); got != "" {
		t.Fatalf("incomplete Bay.ID() = %q, want empty", got)
	}
}

func TestExitCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code ErrorCode
		want int
	}{
		{ErrorNotFound, 3},
		{ErrorUnsupported, 4},
		{ErrorPermission, 5},
		{ErrorAmbiguous, 6},
		{ErrorTimeout, 7},
		{ErrorMalformed, 8},
		{ErrorInternal, 1},
	}

	for _, tt := range tests {
		if got := ExitCode(tt.code); got != tt.want {
			t.Errorf("ExitCode(%q) = %d, want %d", tt.code, got, tt.want)
		}
	}
}
