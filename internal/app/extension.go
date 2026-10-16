package app

import (
	"archive/zip"
	"bytes"
	"io/fs"
	"net/http"
	"sync"

	"github.com/juanrfigueiredo/ligamagic-deck-tracker/extension"
)

var (
	extZipOnce sync.Once
	extZip     []byte
	extZipErr  error
)

// buildExtensionZip empacota a extensão numa pasta mtg-tracker/ dentro do zip.
func buildExtensionZip() ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	err := fs.WalkDir(extension.Files, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(extension.Files, path)
		if err != nil {
			return err
		}
		f, err := zw.Create("mtg-tracker/" + path)
		if err != nil {
			return err
		}
		_, err = f.Write(data)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (a *App) handleExtensionZip(w http.ResponseWriter, r *http.Request) {
	extZipOnce.Do(func() { extZip, extZipErr = buildExtensionZip() })
	if extZipErr != nil {
		http.Error(w, "erro ao gerar o zip", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="mtg-tracker-extensao.zip"`)
	w.Write(extZip)
}
