package webui

import (
	"embed"
	"io/fs"
)

//go:embed index.html app.js temporary.js techobjects.js skz.js styles.css
var embedded embed.FS

func Files() fs.FS {
	return embedded
}
