package dist

import (
	"embed"
	"net/http"
)

//go:embed *.js
//go:embed *.html
var folder embed.FS

var UIHandler = http.FileServer(http.FS(folder))
