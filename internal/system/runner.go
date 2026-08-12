package system

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/zoujunkun/zfs-pay/internal/app"
)

// Command is an argv-only process request. Shell evaluation is intentionally
// not represented by this type.
type Command struct {
	Name string
	Args []string
	Env  []string
}

// Result preserves stdout and stderr separately for structured parsers.
type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// Runner makes all external command execution replaceable in tests.
type Runner interface {
	Run(context.Context, Command) (Result, error)
}

// ExecRunner executes commands directly without a shell.
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, command Command) (Result, error) {
	if command.Name == "" {
		return Result{}, &app.Error{Code: app.ErrorInternal, Op: "exec", Message: "empty command name"}
	}

	cmd := exec.CommandContext(ctx, command.Name, command.Args...)
	cmd.Env = append(os.Environ(), command.Env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if err == nil {
		return result, nil
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return result, &app.Error{Code: app.ErrorTimeout, Op: command.Name, Message: "command timed out", Err: ctx.Err()}
	}
	if errors.Is(err, exec.ErrNotFound) {
		return result, &app.Error{Code: app.ErrorUnsupported, Op: command.Name, Message: "command not found", Err: err}
	}
	if errors.Is(err, os.ErrPermission) || strings.Contains(strings.ToLower(stderr.String()), "permission denied") {
		return result, &app.Error{Code: app.ErrorPermission, Op: command.Name, Message: "permission denied", Err: err}
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, &app.Error{
			Code:    app.ErrorInternal,
			Op:      command.Name,
			Message: fmt.Sprintf("command failed with exit code %d", result.ExitCode),
			Err:     err,
		}
	}
	return result, &app.Error{Code: app.ErrorInternal, Op: command.Name, Message: "command failed", Err: err}
}
