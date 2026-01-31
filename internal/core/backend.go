package core

import "context"

// Backend abstracts the conversation layer (CLI or API)
type Backend interface {
	Converse(ctx context.Context, msg string, responder Responder, perms PermissionChecker) (string, error)
	SessionID() string
	Close() error
}

// BackendFactory creates new Backend instances
type BackendFactory interface {
	Create(workDir string) (Backend, error)
}
