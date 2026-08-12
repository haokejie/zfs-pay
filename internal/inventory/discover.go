package inventory

import (
	"context"
	"path/filepath"

	"github.com/zoujunkun/zfs-pay/internal/app"
	"github.com/zoujunkun/zfs-pay/internal/system"
	"github.com/zoujunkun/zfs-pay/internal/zfs"
)

// ZFSProvider is the read-only leaf-vdev source.
type ZFSProvider interface {
	Discover(context.Context) ([]zfs.Leaf, error)
}

// Snapshot is a complete point-in-time inventory with partial diagnostics.
type Snapshot struct {
	Disks       []app.Disk       `json:"disks"`
	Diagnostics []app.Diagnostic `json:"diagnostics,omitempty"`
}

// Discoverer joins ZFS leaves to Linux block identity.
type Discoverer struct {
	ZFS      ZFSProvider
	Identity system.IdentityProvider
}

func (d Discoverer) Discover(ctx context.Context) (Snapshot, error) {
	leaves, err := d.ZFS.Discover(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot := Snapshot{Disks: make([]app.Disk, 0, len(leaves))}
	paths := make([]string, 0, len(leaves))
	for _, leaf := range leaves {
		if leaf.Path != "" {
			paths = append(paths, leaf.Path)
		}
	}

	identities := map[string]system.Identity{}
	identityDiagnostics := map[string][]app.Diagnostic{}
	if d.Identity != nil && len(paths) > 0 {
		resolved, diagnostics, identityErr := d.Identity.Resolve(ctx, paths)
		if identityErr != nil {
			snapshot.Diagnostics = append(snapshot.Diagnostics, diagnosticFromError("linux", "unable to resolve Linux block identity", identityErr))
		} else {
			identities = resolved
			identityDiagnostics = diagnostics
		}
	}

	for _, leaf := range leaves {
		disk := app.Disk{
			Pool:       leaf.Pool,
			VdevGUID:   leaf.GUID,
			VdevName:   leaf.Name,
			VdevType:   leaf.Role,
			State:      leaf.State,
			DevicePath: leaf.Path,
			Errors:     leaf.Errors,
		}
		if identity, ok := identities[leaf.Path]; ok {
			disk.DevicePath = firstNonEmpty(identity.Device, leaf.Path)
			disk.ParentDevice = identity.ParentDevice
			disk.Serial = identity.Serial
			disk.WWN = identity.WWN
			disk.Model = identity.Model
		}
		disk.Diagnostics = append(disk.Diagnostics, identityDiagnostics[leaf.Path]...)
		if disk.VdevName == "" {
			disk.VdevName = filepath.Base(firstNonEmpty(disk.DevicePath, disk.VdevGUID))
		}
		snapshot.Disks = append(snapshot.Disks, disk)
	}
	return snapshot, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func diagnosticFromError(source, message string, err error) app.Diagnostic {
	code := app.ErrorInternal
	if typed, ok := err.(*app.Error); ok {
		code = typed.Code
	}
	return app.Diagnostic{Code: code, Source: source, Message: message}
}
