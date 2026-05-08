package media

import "strings"

const (
	MaxImageBytes = 10 * 1024 * 1024
	MaxDocBytes   = 50 * 1024 * 1024
)

// SizeCap returns the byte limit for an attachment given its MIME family.
func SizeCap(mimeType string) int {
	if strings.HasPrefix(mimeType, "image/") {
		return MaxImageBytes
	}
	return MaxDocBytes
}
