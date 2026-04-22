package web

import _ "embed"

// indexHTML holds the compiled-in web UI.
// The file is embedded at build time via go:embed.
//
//go:embed static/index.html
var indexHTML string
