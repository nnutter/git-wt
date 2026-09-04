package timber

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseHerdrSpaceRejectsDuplicateFields(t *testing.T) {
	t.Parallel()

	output := []byte(`{"result":{"workspace":{"workspace_id":"w1","workspace_id":"w1"},"tab":{"tab_id":"w1:t1"},"root_pane":{"pane_id":"w1:p1"}}}`)

	_, err := parseHerdrSpace(Runtime{}, output, "/tmp/worktree", "feature")
	require.Error(t, err)
}
