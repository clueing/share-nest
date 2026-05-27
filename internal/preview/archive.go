package preview

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const archiveEntryPreviewLimit = 300

var zipContainerExts = map[string]struct{}{
	".aab":        {},
	".aar":        {},
	".apk":        {},
	".apkm":       {},
	".apks":       {},
	".appx":       {},
	".appxbundle": {},
	".cbz":        {},
	".crx":        {},
	".docm":       {},
	".docx":       {},
	".dotm":       {},
	".dotx":       {},
	".ear":        {},
	".egg":        {},
	".epub":       {},
	".ipa":        {},
	".jar":        {},
	".kmz":        {},
	".kra":        {},
	".key":        {},
	".love":       {},
	".msix":       {},
	".msixbundle": {},
	".numbers":    {},
	".nupkg":      {},
	".odp":        {},
	".odf":        {},
	".odg":        {},
	".ods":        {},
	".odt":        {},
	".ora":        {},
	".otg":        {},
	".otp":        {},
	".ots":        {},
	".ott":        {},
	".pages":      {},
	".ppam":       {},
	".potm":       {},
	".potx":       {},
	".ppsm":       {},
	".ppsx":       {},
	".pptm":       {},
	".pptx":       {},
	".sldm":       {},
	".sldx":       {},
	".sketch":     {},
	".thmx":       {},
	".vsix":       {},
	".war":        {},
	".whl":        {},
	".xapk":       {},
	".xlsb":       {},
	".xlsm":       {},
	".xlsx":       {},
	".xlam":       {},
	".xltm":       {},
	".xltx":       {},
	".xpi":        {},
	".3mf":        {},
}

type ArchiveEntry struct {
	Path         string
	Kind         string
	Size         int64
	SizeKnown    bool
	ModifiedText string
}

type ArchiveSummary struct {
	Format         string
	FormatLabel    string
	EntryCount     int
	FileCount      int
	DirCount       int
	TotalSize      int64
	TotalSizeKnown bool
	Entries        []ArchiveEntry
	Truncated      bool
	Note           string
}

func ArchiveFormat(name, mimeType string) string {
	lowerName := strings.ToLower(strings.TrimSpace(name))
	lowerMIME := strings.ToLower(strings.TrimSpace(mimeType))

	switch {
	case strings.HasSuffix(lowerName, ".tar.gz"), strings.HasSuffix(lowerName, ".tgz"):
		return "tar.gz"
	case strings.HasSuffix(lowerName, ".tar"):
		return "tar"
	case strings.HasSuffix(lowerName, ".zip"):
		return "zip"
	case strings.HasSuffix(lowerName, ".gz"):
		return "gz"
	}

	if _, ok := zipContainerExts[filepath.Ext(lowerName)]; ok {
		return ""
	}

	switch lowerMIME {
	case "application/zip", "application/x-zip-compressed":
		return "zip"
	case "application/x-tar":
		return "tar"
	case "application/gzip", "application/x-gzip":
		return "gz"
	default:
		return ""
	}
}

func ArchiveFormatLabel(format string) string {
	switch format {
	case "zip":
		return "ZIP 压缩包"
	case "tar":
		return "TAR 归档"
	case "tar.gz":
		return "TAR.GZ 压缩包"
	case "gz":
		return "GZIP 压缩文件"
	default:
		return strings.ToUpper(format)
	}
}

func InspectArchive(name, mimeType string, file *os.File, size int64) (*ArchiveSummary, error) {
	format := ArchiveFormat(name, mimeType)
	if format == "" {
		return nil, fmt.Errorf("unsupported archive format")
	}
	if file == nil {
		return nil, fmt.Errorf("archive file is nil")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	summary := &ArchiveSummary{
		Format:         format,
		FormatLabel:    ArchiveFormatLabel(format),
		TotalSizeKnown: true,
	}

	switch format {
	case "zip":
		if err := inspectZip(file, size, summary); err != nil {
			return nil, err
		}
	case "tar":
		if err := inspectTar(file, summary); err != nil {
			return nil, err
		}
	case "tar.gz":
		if err := inspectTarGzip(file, summary); err != nil {
			return nil, err
		}
	case "gz":
		if err := inspectGzip(name, file, size, summary); err != nil {
			return nil, err
		}
	}

	return summary, nil
}

func inspectZip(file *os.File, size int64, summary *ArchiveSummary) error {
	reader, err := zip.NewReader(file, size)
	if err != nil {
		return err
	}

	for _, entry := range reader.File {
		isDir := entry.FileInfo().IsDir()
		summary.EntryCount++
		if isDir {
			summary.DirCount++
		} else {
			summary.FileCount++
			summary.TotalSize += int64(entry.UncompressedSize64)
		}
		summary.appendEntry(ArchiveEntry{
			Path:         normalizeArchivePath(entry.Name),
			Kind:         entryKind(isDir),
			Size:         int64(entry.UncompressedSize64),
			SizeKnown:    !isDir,
			ModifiedText: formatArchiveTime(entry.Modified),
		})
	}
	return nil
}

func inspectTar(file *os.File, summary *ArchiveSummary) error {
	return scanTar(tar.NewReader(file), summary)
}

func inspectTarGzip(file *os.File, summary *ArchiveSummary) error {
	zr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer zr.Close()
	return scanTar(tar.NewReader(zr), summary)
}

func scanTar(reader *tar.Reader, summary *ArchiveSummary) error {
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		isDir := header.FileInfo().IsDir()
		summary.EntryCount++
		if isDir {
			summary.DirCount++
		} else {
			summary.FileCount++
			if header.Size >= 0 {
				summary.TotalSize += header.Size
			}
		}
		summary.appendEntry(ArchiveEntry{
			Path:         normalizeArchivePath(header.Name),
			Kind:         entryKind(isDir),
			Size:         header.Size,
			SizeKnown:    !isDir && header.Size >= 0,
			ModifiedText: formatArchiveTime(header.ModTime),
		})
	}
}

func inspectGzip(name string, file *os.File, size int64, summary *ArchiveSummary) error {
	zr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer zr.Close()

	uncompressedSize, ok := readGzipISize(file, size)
	innerName := strings.TrimSpace(zr.Name)
	if innerName == "" {
		innerName = gzipFallbackName(name)
	}

	summary.EntryCount = 1
	summary.FileCount = 1
	summary.TotalSizeKnown = ok
	if ok {
		summary.TotalSize = uncompressedSize
	}
	summary.Note = "GZIP 通常只包含单个文件，这里展示的是解压后的目标文件信息。"
	summary.appendEntry(ArchiveEntry{
		Path:         normalizeArchivePath(innerName),
		Kind:         "文件",
		Size:         uncompressedSize,
		SizeKnown:    ok,
		ModifiedText: formatArchiveTime(zr.ModTime),
	})
	return nil
}

func readGzipISize(file *os.File, size int64) (int64, bool) {
	if size < 4 {
		return 0, false
	}
	trailer := make([]byte, 4)
	if _, err := file.ReadAt(trailer, size-4); err != nil {
		return 0, false
	}
	return int64(binary.LittleEndian.Uint32(trailer)), true
}

func gzipFallbackName(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"):
		return name[:len(name)-7]
	case strings.HasSuffix(lower, ".tgz"):
		return strings.TrimSuffix(name, filepath.Ext(name)) + ".tar"
	case strings.HasSuffix(lower, ".gz"):
		return strings.TrimSuffix(name, filepath.Ext(name))
	default:
		return name
	}
}

func normalizeArchivePath(path string) string {
	clean := strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	if clean == "" {
		return "(未命名条目)"
	}
	return clean
}

func entryKind(isDir bool) string {
	if isDir {
		return "目录"
	}
	return "文件"
}

func formatArchiveTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Local().Format("2006-01-02 15:04")
}

func (s *ArchiveSummary) appendEntry(entry ArchiveEntry) {
	if len(s.Entries) < archiveEntryPreviewLimit {
		s.Entries = append(s.Entries, entry)
		return
	}
	s.Truncated = true
}
