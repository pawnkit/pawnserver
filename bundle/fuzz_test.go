package bundle

import (
	"os"
	"path/filepath"
	"testing"
)

func FuzzLoadManifest(f *testing.F) {
	f.Add([]byte(`{"schemaVersion":1}`))
	f.Add([]byte(`{}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = Load(data)
	})
}

func FuzzExtract(f *testing.F) {
	f.Add([]byte("not an archive"))

	f.Fuzz(func(t *testing.T, data []byte) {
		root := t.TempDir()
		archive := filepath.Join(root, "input.pawnbundle")

		if err := os.WriteFile(archive, data, 0o600); err != nil {
			t.Fatal(err)
		}

		_ = Extract(archive, filepath.Join(root, "output"))
	})
}
