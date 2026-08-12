package app

import "fmt"

// ErrorCode is stable for CLI exit-code and JSON diagnostic mapping.
type ErrorCode string

const (
	ErrorUnsupported ErrorCode = "unsupported"
	ErrorPermission  ErrorCode = "permission_denied"
	ErrorTimeout     ErrorCode = "timeout"
	ErrorAmbiguous   ErrorCode = "ambiguous"
	ErrorMalformed   ErrorCode = "malformed_output"
	ErrorNotFound    ErrorCode = "not_found"
	ErrorInternal    ErrorCode = "internal"
)

// Error is the typed application error shared by providers and the CLI.
type Error struct {
	Code    ErrorCode
	Op      string
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Op == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Op, e.Message)
}

func (e *Error) Unwrap() error { return e.Err }

// ExitCode maps application failures to stable process exit codes.
func ExitCode(code ErrorCode) int {
	switch code {
	case ErrorNotFound:
		return 3
	case ErrorUnsupported:
		return 4
	case ErrorPermission:
		return 5
	case ErrorAmbiguous:
		return 6
	case ErrorTimeout:
		return 7
	case ErrorMalformed:
		return 8
	default:
		return 1
	}
}
