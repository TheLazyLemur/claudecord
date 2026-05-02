package permission

import (
	"path/filepath"
	"strings"

	"github.com/TheLazyLemur/claudecord/internal/core"
)

var _ core.PermissionChecker = (*MediaAwarePermissionChecker)(nil)

// MediaAwarePermissionChecker auto-approves Read calls under mediaDir and
// delegates everything else to the wrapped checker.
//
// Rationale: WhatsApp users explicitly send the file by uploading it; prompting
// "approve reading /path/foo.jpg?" once per attachment in a 5-photo burst makes
// the bot effectively unusable. The carve-out is safe because mediaDir is
// validated at startup to live inside ALLOWED_DIRS.
type MediaAwarePermissionChecker struct {
	inner    core.PermissionChecker
	mediaDir string
}

func NewMediaAwarePermissionChecker(inner core.PermissionChecker, mediaDir string) *MediaAwarePermissionChecker {
	return &MediaAwarePermissionChecker{
		inner:    inner,
		mediaDir: filepath.Clean(mediaDir),
	}
}

func (m *MediaAwarePermissionChecker) Check(toolName string, input core.ToolInput) (bool, string) {
	if toolName == "Read" && m.mediaDir != "" && pathUnder(input.FilePath, m.mediaDir) {
		return true, ""
	}
	return m.inner.Check(toolName, input)
}

func pathUnder(path, dir string) bool {
	if path == "" {
		return false
	}
	clean := filepath.Clean(path)
	return clean == dir || strings.HasPrefix(clean, dir+string(filepath.Separator))
}
