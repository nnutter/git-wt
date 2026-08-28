package herdr

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestEmbeddedPluginMatchesSourceFiles(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"herdr-plugin.toml", "bin/create", "bin/open"} {
		want, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		got, err := pluginFiles.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Fatalf("%s: embedded contents differ from source", name)
		}
	}
}

func TestWritePluginReplacesDestinationAndPreservesExecutables(t *testing.T) {
	t.Parallel()

	destination := filepath.Join(t.TempDir(), "timber")
	stalePath := filepath.Join(destination, "stale")
	if err := os.MkdirAll(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stalePath, []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := WritePlugin(destination); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(stalePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale file still present: %v", err)
	}

	for _, name := range []string{"herdr-plugin.toml", "bin/create", "bin/open"} {
		got, err := os.ReadFile(filepath.Join(destination, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		want, err := pluginFiles.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Fatalf("%s: installed contents differ from embed", name)
		}
	}

	createInfo, err := os.Stat(filepath.Join(destination, "bin", "create"))
	if err != nil {
		t.Fatal(err)
	}
	if createInfo.Mode()&0o111 == 0 {
		t.Fatal("bin/create is not executable")
	}
}
