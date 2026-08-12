package system

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zoujunkun/zfs-pay/internal/app"
)

func TestExecRunnerClassifiesMissingCommand(t *testing.T) {
	t.Parallel()

	_, err := (ExecRunner{}).Run(context.Background(), Command{Name: "zfs-pay-command-that-does-not-exist"})
	var typed *app.Error
	if !errors.As(err, &typed) || typed.Code != app.ErrorUnsupported {
		t.Fatalf("error = %#v, want unsupported", err)
	}
}

func TestExecRunnerHonorsContextTimeout(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := (ExecRunner{}).Run(ctx, Command{Name: "sleep", Args: []string{"1"}})
	var typed *app.Error
	if !errors.As(err, &typed) || typed.Code != app.ErrorTimeout {
		t.Fatalf("error = %#v, want timeout", err)
	}
}

func TestExecRunnerCapturesExitCode(t *testing.T) {
	t.Parallel()

	result, err := (ExecRunner{}).Run(context.Background(), Command{Name: "false"})
	if err == nil || result.ExitCode != 1 {
		t.Fatalf("result = %#v, error = %v", result, err)
	}
}
