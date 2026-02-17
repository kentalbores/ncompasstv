// Package media provides centralized media type detection
// for the player, distinguishing between video and image content.
package media

import (
	"path/filepath"
	"strings"
)

// Type represents the kind of media file.
type Type int

const (
	Unknown Type = iota
	Video
	Image
)

func (t Type) String() string {
	switch t {
	case Video:
		return "video"
	case Image:
		return "image"
	default:
		return "unknown"
	}
}

// Video file extensions.
var videoExts = map[string]bool{
	".mp4":  true,
	".mkv":  true,
	".avi":  true,
	".mov":  true,
	".webm": true,
	".ts":   true,
	".m4v":  true,
	".hevc": true,
	".flv":  true,
	".wmv":  true,
}

// Image file extensions.
var imageExts = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".bmp":  true,
	".gif":  true,
	".webp": true,
	".tiff": true,
	".svg":  true,
}

// Detect returns the media type for a given file path based on extension.
func Detect(path string) Type {
	ext := strings.ToLower(filepath.Ext(path))
	if videoExts[ext] {
		return Video
	}
	if imageExts[ext] {
		return Image
	}
	return Unknown
}

// IsSupported returns true if the file has a recognized media extension.
func IsSupported(path string) bool {
	return Detect(path) != Unknown
}

// DefaultImageDuration is how long (in seconds) an image is displayed
// before advancing to the next item in the playlist.
const DefaultImageDuration = 10
