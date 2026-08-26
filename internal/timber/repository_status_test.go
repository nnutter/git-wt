package timber

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePorcelainStatus(t *testing.T) {
	tests := []struct {
		name   string
		output string
		status string
		clean  bool
	}{
		{
			name:   "clean worktree",
			output: "# branch.oid abc123\n# branch.head feature/one\n# branch.upstream origin/main\n# branch.ab +0 -0",
			status: "[origin/main]",
			clean:  true,
		},
		{
			name:   "ahead and behind upstream",
			output: "# branch.oid abc123\n# branch.head feature/one\n# branch.upstream origin/dev\n# branch.ab +18 -62",
			status: "↑18 ↓62 [origin/dev]",
			clean:  true,
		},
		{
			name:   "ahead of upstream",
			output: "# branch.upstream origin/main\n# branch.ab +3 -0",
			status: "↑3 [origin/main]",
			clean:  true,
		},
		{
			name:   "behind upstream",
			output: "# branch.upstream origin/main\n# branch.ab +0 -4",
			status: "↓4 [origin/main]",
			clean:  true,
		},
		{
			name:   "no upstream",
			output: "# branch.oid abc123\n# branch.head feature/one",
			status: "",
			clean:  true,
		},
		{
			name:   "missing upstream ref",
			output: "# branch.oid abc123\n# branch.head feature/one\n# branch.upstream origin/main",
			status: "[origin/main]",
			clean:  true,
		},
		{
			name:   "modified file",
			output: "# branch.upstream origin/main\n# branch.ab +0 -0\n1 .M N... 100644 100644 100644 abc123 abc123 file.txt",
			status: "[origin/main]",
			clean:  false,
		},
		{
			name:   "untracked file",
			output: "# branch.upstream origin/main\n# branch.ab +0 -0\n? new.txt",
			status: "[origin/main]",
			clean:  false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			status, clean, err := parsePorcelainStatus(testCase.output)
			require.NoError(t, err)
			assert.Equal(t, testCase.status, status)
			assert.Equal(t, testCase.clean, clean)
		})
	}
}
