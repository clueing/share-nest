package storage

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"file-service/internal/security"
)

type Local struct {
	root string
}

func NewLocal(root string) (*Local, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &Local{root: root}, nil
}

func (l *Local) SaveUploadedFile(file io.Reader, originalName string) (relativePath, mimeType, shaValue string, size int64, err error) {
	head := make([]byte, 512)
	n, readErr := file.Read(head)
	if readErr != nil && readErr != io.EOF {
		return "", "", "", 0, readErr
	}

	now := time.Now()
	subdir := filepath.Join("uploads", now.Format("2006"), now.Format("01"))
	if err := os.MkdirAll(filepath.Join(l.root, subdir), 0o755); err != nil {
		return "", "", "", 0, err
	}

	ext := strings.ToLower(filepath.Ext(originalName))
	filenameBase, err := security.RandomString(16)
	if err != nil {
		return "", "", "", 0, err
	}
	filename := filenameBase + ext
	relativePath = filepath.ToSlash(filepath.Join(subdir, filename))
	fullPath := filepath.Join(l.root, filepath.FromSlash(relativePath))

	dst, err := os.Create(fullPath)
	if err != nil {
		return "", "", "", 0, err
	}
	defer dst.Close()

	hash := sha256.New()
	reader := io.MultiReader(bytes.NewReader(head[:n]), file)
	written, err := io.Copy(io.MultiWriter(dst, hash), reader)
	if err != nil {
		return "", "", "", 0, err
	}

	mimeType = http.DetectContentType(head[:n])
	if override, ok := mimeByExtension(ext); ok {
		mimeType = override
	}
	return relativePath, mimeType, hex.EncodeToString(hash.Sum(nil)), written, nil
}

func mimeByExtension(ext string) (string, bool) {
	switch ext {
	case ".svg":
		return "image/svg+xml", true
	case ".mp3":
		return "audio/mpeg", true
	case ".wav":
		return "audio/wav", true
	case ".ogg", ".oga":
		return "audio/ogg", true
	case ".m4a":
		return "audio/mp4", true
	case ".aac":
		return "audio/aac", true
	case ".flac":
		return "audio/flac", true
	case ".opus":
		return "audio/opus", true
	case ".mp4", ".m4v":
		return "video/mp4", true
	case ".webm":
		return "video/webm", true
	case ".ogv":
		return "video/ogg", true
	case ".mov":
		return "video/quicktime", true
	case ".apk":
		return "application/vnd.android.package-archive", true
	case ".aab", ".apks", ".apkm", ".xapk", ".ipa":
		return "application/octet-stream", true
	case ".aar", ".jar", ".war", ".ear":
		return "application/java-archive", true
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document", true
	case ".docm":
		return "application/vnd.ms-word.document.macroenabled.12", true
	case ".dotx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.template", true
	case ".dotm":
		return "application/vnd.ms-word.template.macroenabled.12", true
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", true
	case ".xlsm":
		return "application/vnd.ms-excel.sheet.macroenabled.12", true
	case ".xlsb":
		return "application/vnd.ms-excel.sheet.binary.macroenabled.12", true
	case ".xlam":
		return "application/vnd.ms-excel.addin.macroenabled.12", true
	case ".xltx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.template", true
	case ".xltm":
		return "application/vnd.ms-excel.template.macroenabled.12", true
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation", true
	case ".pptm":
		return "application/vnd.ms-powerpoint.presentation.macroenabled.12", true
	case ".potx":
		return "application/vnd.openxmlformats-officedocument.presentationml.template", true
	case ".potm":
		return "application/vnd.ms-powerpoint.template.macroenabled.12", true
	case ".ppsx":
		return "application/vnd.openxmlformats-officedocument.presentationml.slideshow", true
	case ".ppsm":
		return "application/vnd.ms-powerpoint.slideshow.macroenabled.12", true
	case ".ppam":
		return "application/vnd.ms-powerpoint.addin.macroenabled.12", true
	case ".sldx":
		return "application/vnd.openxmlformats-officedocument.presentationml.slide", true
	case ".sldm":
		return "application/vnd.ms-powerpoint.slide.macroenabled.12", true
	case ".thmx":
		return "application/vnd.ms-officetheme", true
	case ".odt":
		return "application/vnd.oasis.opendocument.text", true
	case ".ott":
		return "application/vnd.oasis.opendocument.text-template", true
	case ".ods":
		return "application/vnd.oasis.opendocument.spreadsheet", true
	case ".ots":
		return "application/vnd.oasis.opendocument.spreadsheet-template", true
	case ".odp":
		return "application/vnd.oasis.opendocument.presentation", true
	case ".otp":
		return "application/vnd.oasis.opendocument.presentation-template", true
	case ".odg":
		return "application/vnd.oasis.opendocument.graphics", true
	case ".otg":
		return "application/vnd.oasis.opendocument.graphics-template", true
	case ".odf":
		return "application/vnd.oasis.opendocument.formula", true
	case ".epub":
		return "application/epub+zip", true
	case ".kmz":
		return "application/vnd.google-earth.kmz", true
	case ".3mf":
		return "model/3mf", true
	case ".vsix":
		return "application/vsix", true
	case ".xpi":
		return "application/x-xpinstall", true
	case ".crx":
		return "application/x-chrome-extension", true
	case ".key":
		return "application/vnd.apple.keynote", true
	case ".numbers":
		return "application/vnd.apple.numbers", true
	case ".pages":
		return "application/vnd.apple.pages", true
	case ".appx", ".appxbundle", ".msix", ".msixbundle", ".nupkg", ".whl", ".egg", ".love", ".sketch", ".kra", ".ora":
		return "application/octet-stream", true
	case ".cbz":
		return "application/vnd.comicbook+zip", true
	case ".zip":
		return "application/zip", true
	case ".tar":
		return "application/x-tar", true
	case ".tgz", ".gz":
		return "application/gzip", true
	default:
		return "", false
	}
}

func (l *Local) Open(relativePath string) (*os.File, error) {
	fullPath, err := l.resolvePath(relativePath)
	if err != nil {
		return nil, err
	}
	return os.Open(fullPath)
}

func (l *Local) Remove(relativePath string) error {
	fullPath, err := l.resolvePath(relativePath)
	if err != nil {
		return err
	}
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	_ = l.cleanupEmptyDirs(filepath.Dir(fullPath))
	return nil
}

func (l *Local) ReadText(relativePath string, limit int64) (string, bool, error) {
	file, err := l.Open(relativePath)
	if err != nil {
		return "", false, err
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return "", false, err
	}

	truncated := int64(len(data)) > limit
	if truncated {
		data = data[:limit]
	}
	return string(data), truncated, nil
}

func (l *Local) resolvePath(relativePath string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(relativePath))
	fullPath := filepath.Join(l.root, clean)
	rootClean := filepath.Clean(l.root)

	rel, err := filepath.Rel(rootClean, fullPath)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid file path")
	}
	return fullPath, nil
}

func (l *Local) cleanupEmptyDirs(dir string) error {
	rootClean := filepath.Clean(l.root)
	current := filepath.Clean(dir)

	for current != rootClean {
		entries, err := os.ReadDir(current)
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			return nil
		}
		if err := os.Remove(current); err != nil {
			return err
		}
		current = filepath.Dir(current)
	}
	return nil
}
