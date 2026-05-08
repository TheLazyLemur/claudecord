package core

import "context"

// Backend abstracts the conversation layer (CLI or API)
type Backend interface {
	Converse(ctx context.Context, in Inbound, out Outbound, perms PermissionChecker) (string, error)
	SessionID() string
	Close() error
}

// BackendFactory creates new Backend instances.
// caps describes the active plugin's capabilities (e.g. Reactions) and is
// used to gate per-session tool registration.
type BackendFactory interface {
	Create(workDir string, caps Capabilities) (Backend, error)
}
