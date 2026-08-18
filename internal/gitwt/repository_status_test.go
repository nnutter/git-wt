package gitwt

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
			output: "## feature/one...origin/feature/one",
			status: "feature/one...origin/feature/one",
			clean:  true,
		},
		{
			name:   "modified file",
			output: "## feature/one...origin/feature/one\n M file.txt",
			status: "feature/one...origin/feature/one",
			clean:  false,
		},
		{
			name:   "untracked file",
			output: "## feature/one...origin/feature/one\n?? new.txt",
			status: "feature/one...origin/feature/one",
			clean:  false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			status, clean := parsePorcelainStatus(testCase.output)

			assert.Equal(t, testCase.status, status)
			assert.Equal(t, testCase.clean, clean)
		})
	}
}
