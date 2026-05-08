package core

import "context"

// Backend abstracts the conversation layer (CLI or API)
type Backend interface {
	Converse(ctx context.Context, msg string, out Outbound, perms PermissionChecker) (string, error)
	SessionID() string
	Close() error
}

// BackendFactory creates new Backend instances
type BackendFactory interface {
	Create(workDir string) (Backend, error)
}
