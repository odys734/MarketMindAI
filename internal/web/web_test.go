package web

import (
	"io/fs"
	"testing"
)

func TestFilesystemContainsDashboard(t *testing.T) {
	files := Filesystem()
	for _, name := range []string{"index.html", "app.js", "styles.css", "manifest.webmanifest", "sw.js", "research-lab.js", "research-lab.css"} {
		if _, err := fs.Stat(files, name); err != nil {
			t.Fatalf("embedded dashboard asset %q is missing: %v", name, err)
		}
	}
}
