package herdr

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed herdr-plugin.toml bin
var pluginFiles embed.FS

func WritePlugin(destination string) error {
	if err := os.RemoveAll(destination); err != nil {
		return fmt.Errorf("remove existing herdr plugin: %w", err)
	}

	return fs.WalkDir(pluginFiles, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		target := destination
		if path != "." {
			target = filepath.Join(destination, filepath.FromSlash(path))
		}
		if entry.IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("create herdr plugin directory %q: %w", target, err)
			}
			return nil
		}

		data, err := pluginFiles.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read embedded herdr plugin file %q: %w", path, err)
		}
		if err := os.WriteFile(target, data, pluginFileMode(path)); err != nil {
			return fmt.Errorf("write herdr plugin file %q: %w", target, err)
		}
		return nil
	})
}

func pluginFileMode(path string) os.FileMode {
	if strings.HasPrefix(filepath.ToSlash(path), "bin/") {
		return 0o755
	}
	return 0o644
}
