package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/zoujunkun/zfs-pay/internal/app"
)

const SchemaVersion = 1

// Mapping is a physical location verified from a live disk identity.
type Mapping struct {
	Pool         string    `json:"pool"`
	GUID         string    `json:"guid"`
	Serial       string    `json:"serial,omitempty"`
	WWN          string    `json:"wwn,omitempty"`
	Bay          app.Bay   `json:"bay"`
	LastVerified time.Time `json:"last_verified"`
}

// ManagedLED records locate LEDs switched on by zfs-pay itself.
type ManagedLED struct {
	Pool  string    `json:"pool"`
	GUID  string    `json:"guid"`
	Bay   app.Bay   `json:"bay"`
	Since time.Time `json:"since"`
}

// Data is the versioned on-disk state. Maps make updates idempotent.
type Data struct {
	SchemaVersion int                   `json:"schema_version"`
	Mappings      map[string]Mapping    `json:"mappings"`
	ManagedLEDs   map[string]ManagedLED `json:"managed_leds"`
	LastEvents    map[string]string     `json:"last_events"`
}

func NewData() Data {
	return Data{
		SchemaVersion: SchemaVersion,
		Mappings:      make(map[string]Mapping),
		ManagedLEDs:   make(map[string]ManagedLED),
		LastEvents:    make(map[string]string),
	}
}

// Store serializes readers and writers with flock and persists through an
// fsync/rename sequence in the destination directory.
type Store struct {
	Path string
	Now  func() time.Time
}

func (s Store) Read(ctx context.Context) (Data, error) {
	var data Data
	err := s.withLock(ctx, func() error {
		loaded, err := s.loadUnlocked()
		data = loaded
		return err
	})
	return data, err
}

// Update recovers a malformed state file by preserving it with a .corrupt
// suffix before applying the callback to a clean state.
func (s Store) Update(ctx context.Context, update func(*Data) error) (bool, error) {
	recovered := false
	err := s.withLock(ctx, func() error {
		data, err := s.loadUnlocked()
		if err != nil {
			var typed *app.Error
			if !errors.As(err, &typed) || typed.Code != app.ErrorMalformed {
				return err
			}
			if renameErr := s.quarantineUnlocked(); renameErr != nil {
				return renameErr
			}
			data = NewData()
			recovered = true
		}
		if err := update(&data); err != nil {
			return err
		}
		return s.writeUnlocked(data)
	})
	return recovered, err
}

func (s Store) loadUnlocked() (Data, error) {
	data := NewData()
	raw, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return data, nil
	}
	if err != nil {
		return Data{}, &app.Error{Code: app.ErrorPermission, Op: "read state", Message: "unable to read state file", Err: err}
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return Data{}, &app.Error{Code: app.ErrorMalformed, Op: "read state", Message: "invalid state JSON", Err: err}
	}
	if data.SchemaVersion != SchemaVersion {
		return Data{}, &app.Error{Code: app.ErrorMalformed, Op: "read state", Message: fmt.Sprintf("unsupported schema version %d", data.SchemaVersion)}
	}
	if data.Mappings == nil {
		data.Mappings = make(map[string]Mapping)
	}
	if data.ManagedLEDs == nil {
		data.ManagedLEDs = make(map[string]ManagedLED)
	}
	if data.LastEvents == nil {
		data.LastEvents = make(map[string]string)
	}
	return data, nil
}

func (s Store) writeUnlocked(data Data) error {
	data.SchemaVersion = SchemaVersion
	dir := filepath.Dir(s.Path)
	file, err := os.CreateTemp(dir, ".zfs-pay-state-*")
	if err != nil {
		return &app.Error{Code: app.ErrorPermission, Op: "write state", Message: "unable to create temporary state", Err: err}
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return &app.Error{Code: app.ErrorPermission, Op: "write state", Message: "unable to secure temporary state", Err: err}
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		file.Close()
		return &app.Error{Code: app.ErrorInternal, Op: "write state", Message: "unable to encode state", Err: err}
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return &app.Error{Code: app.ErrorInternal, Op: "write state", Message: "unable to sync state", Err: err}
	}
	if err := file.Close(); err != nil {
		return &app.Error{Code: app.ErrorInternal, Op: "write state", Message: "unable to close state", Err: err}
	}
	if err := os.Rename(temporary, s.Path); err != nil {
		return &app.Error{Code: app.ErrorPermission, Op: "write state", Message: "unable to replace state", Err: err}
	}
	directory, err := os.Open(dir)
	if err == nil {
		defer directory.Close()
		if syncErr := directory.Sync(); syncErr != nil {
			return &app.Error{Code: app.ErrorInternal, Op: "write state", Message: "unable to sync state directory", Err: syncErr}
		}
	}
	return nil
}

func (s Store) quarantineUnlocked() error {
	if _, err := os.Stat(s.Path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	backup := fmt.Sprintf("%s.corrupt-%d", s.Path, s.now().UnixNano())
	if err := os.Rename(s.Path, backup); err != nil {
		return &app.Error{Code: app.ErrorPermission, Op: "recover state", Message: "unable to preserve corrupt state", Err: err}
	}
	return nil
}

func (s Store) withLock(ctx context.Context, run func() error) error {
	if s.Path == "" {
		return &app.Error{Code: app.ErrorMalformed, Op: "state lock", Message: "state path is empty"}
	}
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return &app.Error{Code: app.ErrorPermission, Op: "state lock", Message: "unable to create state directory", Err: err}
	}
	lock, err := os.OpenFile(s.Path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return &app.Error{Code: app.ErrorPermission, Op: "state lock", Message: "unable to open lock file", Err: err}
	}
	defer lock.Close()
	for {
		err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			return &app.Error{Code: app.ErrorPermission, Op: "state lock", Message: "unable to acquire lock", Err: err}
		}
		select {
		case <-ctx.Done():
			return &app.Error{Code: app.ErrorTimeout, Op: "state lock", Message: "timed out waiting for lock", Err: ctx.Err()}
		case <-time.After(25 * time.Millisecond):
		}
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	return run()
}

func (s Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}
