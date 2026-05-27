package storage

import "testing"

func TestMimeByExtensionOverridesZipContainers(t *testing.T) {
	tests := map[string]string{
		".apk":   "application/vnd.android.package-archive",
		".docx":  "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		".xlsx":  "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		".pptx":  "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		".odt":   "application/vnd.oasis.opendocument.text",
		".odg":   "application/vnd.oasis.opendocument.graphics",
		".epub":  "application/epub+zip",
		".kmz":   "application/vnd.google-earth.kmz",
		".3mf":   "model/3mf",
		".vsix":  "application/vsix",
		".crx":   "application/x-chrome-extension",
		".xpi":   "application/x-xpinstall",
		".pages": "application/vnd.apple.pages",
		".cbz":   "application/vnd.comicbook+zip",
	}

	for ext, want := range tests {
		t.Run(ext, func(t *testing.T) {
			got, ok := mimeByExtension(ext)
			if !ok {
				t.Fatalf("mimeByExtension(%q) did not return an override", ext)
			}
			if got != want {
				t.Fatalf("mimeByExtension(%q) = %q, want %q", ext, got, want)
			}
		})
	}
}

func TestMimeByExtensionKeepsArchives(t *testing.T) {
	tests := map[string]string{
		".zip": "application/zip",
		".tar": "application/x-tar",
		".tgz": "application/gzip",
		".gz":  "application/gzip",
	}

	for ext, want := range tests {
		t.Run(ext, func(t *testing.T) {
			got, ok := mimeByExtension(ext)
			if !ok {
				t.Fatalf("mimeByExtension(%q) did not return an override", ext)
			}
			if got != want {
				t.Fatalf("mimeByExtension(%q) = %q, want %q", ext, got, want)
			}
		})
	}
}
