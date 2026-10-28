package web

import (
	"encoding/json"
	"io/fs"
)

const manifestPath = "manifest.webmanifest"

func darkManifest(static fs.FS) []byte {
	b, err := fs.ReadFile(static, manifestPath)
	if err != nil {
		panic(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		panic(err)
	}
	m["theme_color"], m["background_color"] = "#181b20", "#181b20"
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		panic(err)
	}
	return out
}
