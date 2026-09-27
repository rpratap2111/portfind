// Package assets embeds files shipped inside the portfind binaries.
package assets

import _ "embed"

// Icon is the portfind icon (multi-size .ico, 16–256 px), used by the tray.
//
//go:embed portfind.ico
var Icon []byte
