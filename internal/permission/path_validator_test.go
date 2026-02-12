package permission

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPathValidator_ExtractPaths(t *testing.T) {
	a := assert.New(t)

	v := NewPathValidator([]string{"/home"})

	// extracts all known path fields
	paths := v.ExtractPaths(map[string]any{
		"file_path": "/home/a.go",
		"path":      "/home/b.go",
		"directory": "/home/c",
	})
	a.ElementsMatch([]string{"/home/a.go", "/home/b.go", "/home/c"}, paths)

	// skips empty/missing
	paths = v.ExtractPaths(map[string]any{"file_path": "", "other": "val"})
	a.Empty(paths)
}

func TestPathValidator_IsAllowed(t *testing.T) {
	a := assert.New(t)

	v := NewPathValidator([]string{"/home/user/projects", "/opt/data"})

	// allowed - exact match
	a.True(v.IsAllowed("/home/user/projects"))

	// allowed - subdirectory
	a.True(v.IsAllowed("/home/user/projects/deep/file.go"))

	// allowed - second dir
	a.True(v.IsAllowed("/opt/data/file.csv"))

	// denied - outside
	a.False(v.IsAllowed("/etc/passwd"))

	// denied - path traversal
	a.False(v.IsAllowed("/home/user/projects/../../../etc/passwd"))

	// denied - prefix trick (e.g. /home/user/projects-evil)
	a.False(v.IsAllowed("/home/user/projects-evil/file.go"))
}

func TestPathValidator_CleansAllowedDirs(t *testing.T) {
	a := assert.New(t)

	// trailing slash gets cleaned
	v := NewPathValidator([]string{"/home/user/projects/"})
	a.True(v.IsAllowed("/home/user/projects/file.go"))
}
