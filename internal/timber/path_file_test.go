package timber

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWritePathFileRejectsPathOutsideTemporaryDirectory(t *testing.T) {
	t.Parallel()
	pathFile := filepath.Join(os.TempDir(), "..", "timber-path-file")

	runtime := testRuntime(t)
	err := runtime.writePathFile(pathFile, "worktree")

	require.Error(t, err)
	require.Contains(t, err.Error(), "outside temporary directory")
}
