// Package extension embute os arquivos da extensão para o site oferecer o download.
package extension

import "embed"

//go:embed manifest.json *.js popup.html
var Files embed.FS
