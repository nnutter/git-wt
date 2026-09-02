package timber

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func writePathFile(pathFile string, value string) (err error) {
	temporaryDirectory, err := filepath.Abs(os.TempDir())
	if err != nil {
		return fmt.Errorf("resolve temporary directory: %w", err)
	}
	pathFile, err = filepath.Abs(pathFile)
	if err != nil {
		return fmt.Errorf("resolve path file: %w", err)
	}

	relativePath, err := filepath.Rel(temporaryDirectory, pathFile)
	if err != nil {
		return fmt.Errorf("relate path file to temporary directory: %w", err)
	}
	if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path file %q is outside temporary directory", pathFile)
	}

	root, err := os.OpenRoot(temporaryDirectory)
	if err != nil {
		return fmt.Errorf("open temporary directory: %w", err)
	}
	defer func() {
		if closeErr := root.Close(); err == nil {
			err = closeErr
		}
	}()

	return root.WriteFile(relativePath, []byte(value+"\n"), 0o600)
}
