package e2e

import (
	"testing"
)

func TestList(t *testing.T) {
	setupTestRepo(t)

	if err := executeCmd(t, "create", "list-test-1"); err != nil {
		t.Fatalf("create list-test-1: %v", err)
	}
	if err := executeCmd(t, "create", "nn/list-test-2"); err != nil {
		t.Fatalf("create nn/list-test-2: %v", err)
	}

	if err := executeCmd(t, "list"); err != nil {
		t.Fatalf("list: %v", err)
	}
}

func TestListEmpty(t *testing.T) {
	setupTestRepo(t)

	if err := executeCmd(t, "list"); err != nil {
		t.Fatalf("list with no worktrees: %v", err)
	}
}
