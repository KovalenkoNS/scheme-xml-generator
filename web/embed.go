// Package webui embeds the application shell and responsibility-separated UI components.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed index.html shared shell sources equipment generation library preview
var embedded embed.FS

// Files supplies the local HTTP server with the shell, library editor and XML preview.
// It returns embedded assets only; user sources and generated XML are served separately.
func Files() fs.FS {
	return embedded
}
