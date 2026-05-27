package preview

import "testing"

func TestDetectSkipsZipContainerExtensions(t *testing.T) {
	tests := []string{
		"app.apk",
		"document.docx",
		"sheet.xlsx",
		"slides.pptx",
		"book.epub",
		"package.ipa",
		"extension.vsix",
		"package.nupkg",
	}

	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			if got := Detect("file", "application/zip", name); got == ModeArchive {
				t.Fatalf("Detect(file, application/zip, %q) = %q, want non-archive", name, got)
			}
		})
	}
}

func TestDetectKeepsZipArchives(t *testing.T) {
	if got := Detect("file", "application/octet-stream", "archive.zip"); got != ModeArchive {
		t.Fatalf("Detect archive.zip = %q, want %q", got, ModeArchive)
	}
	if got := Detect("file", "application/zip", "archive"); got != ModeArchive {
		t.Fatalf("Detect extensionless zip = %q, want %q", got, ModeArchive)
	}
}
