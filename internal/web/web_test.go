package web

import (
	"io/fs"
	"testing"
)

func TestFilesystemContainsDashboard(t *testing.T) {
	files := Filesystem()
	for _, name := range []string{"index.html", "app.js", "styles.css", "manifest.webmanifest", "sw.js"} {
		if _, err := fs.Stat(files, name); err != nil {
			t.Fatalf("embedded dashboard asset %q is missing: %v", name, err)
		}
	}
}
