package preview

import (
	"path/filepath"
	"strings"
)

type Mode string

const (
	ModeNone  Mode = "none"
	ModeText  Mode = "text"
	ModeImage Mode = "image"
	ModeAudio Mode = "audio"
	ModeVideo Mode = "video"
	ModePDF   Mode = "pdf"
)

var textExts = map[string]struct{}{
	"txt":  {},
	"md":   {},
	"json": {},
	"yaml": {},
	"yml":  {},
	"xml":  {},
	"csv":  {},
	"log":  {},
	"ini":  {},
	"toml": {},
	"go":   {},
	"js":   {},
	"mjs":  {},
	"ts":   {},
	"jsx":  {},
	"tsx":  {},
	"py":   {},
	"java": {},
	"c":    {},
	"cc":   {},
	"cpp":  {},
	"h":    {},
	"hpp":  {},
	"rs":   {},
	"sh":   {},
	"sql":  {},
	"html": {},
	"htm":  {},
	"css":  {},
	"svg":  {},
}

func Detect(kind, mimeType, name string) Mode {
	if kind == "text" {
		return ModeText
	}

	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	mimeType = strings.ToLower(mimeType)

	if _, ok := textExts[ext]; ok {
		return ModeText
	}
	if strings.HasPrefix(mimeType, "text/") {
		return ModeText
	}
	if strings.HasPrefix(mimeType, "image/") {
		return ModeImage
	}
	if strings.HasPrefix(mimeType, "audio/") {
		return ModeAudio
	}
	if strings.HasPrefix(mimeType, "video/") {
		return ModeVideo
	}
	if mimeType == "application/pdf" || ext == "pdf" {
		return ModePDF
	}
	return ModeNone
}
