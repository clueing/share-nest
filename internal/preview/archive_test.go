package preview

import "testing"

func TestArchiveFormatSkipsZipContainerExtensions(t *testing.T) {
	tests := []string{
		"app.apk",
		"bundle.aab",
		"library.aar",
		"plugin.jar",
		"site.war",
		"book.epub",
		"document.docx",
		"document.docm",
		"sheet.xlsx",
		"sheet.xlsm",
		"macro.xlam",
		"slides.pptx",
		"slides.pptm",
		"theme.thmx",
		"text.odt",
		"table.ods",
		"deck.odp",
		"drawing.odg",
		"app.ipa",
		"extension.vsix",
		"extension.crx",
		"addon.xpi",
		"package.nupkg",
		"wheel.whl",
		"map.kmz",
		"comic.cbz",
		"design.sketch",
		"model.3mf",
		"document.pages",
	}

	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			if got := ArchiveFormat(name, "application/zip"); got != "" {
				t.Fatalf("ArchiveFormat(%q, application/zip) = %q, want empty", name, got)
			}
		})
	}
}

func TestArchiveFormatKeepsRealArchives(t *testing.T) {
	tests := []struct {
		name     string
		mimeType string
		want     string
	}{
		{name: "archive.zip", mimeType: "", want: "zip"},
		{name: "archive.tar", mimeType: "", want: "tar"},
		{name: "archive.tar.gz", mimeType: "", want: "tar.gz"},
		{name: "archive.tgz", mimeType: "", want: "tar.gz"},
		{name: "archive.gz", mimeType: "", want: "gz"},
		{name: "archive", mimeType: "application/zip", want: "zip"},
		{name: "archive", mimeType: "application/x-tar", want: "tar"},
		{name: "archive", mimeType: "application/gzip", want: "gz"},
	}

	for _, tt := range tests {
		t.Run(tt.name+"/"+tt.mimeType, func(t *testing.T) {
			if got := ArchiveFormat(tt.name, tt.mimeType); got != tt.want {
				t.Fatalf("ArchiveFormat(%q, %q) = %q, want %q", tt.name, tt.mimeType, got, tt.want)
			}
		})
	}
}
