package turboist

import "embed"

//go:embed all:frontend/build
var StaticFS embed.FS
