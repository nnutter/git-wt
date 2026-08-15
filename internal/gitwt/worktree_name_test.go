package gitwt

import (
	"errors"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateGeneratesRandomNameWhenOmitted(t *testing.T) {
	testRepository := newTestRepository(t)

	result := testRepository.runGitWT(t, "create", "--repo", testRepoName)
	require.NoError(t, result.err, result.stderr)

	worktreePath := strings.TrimSpace(result.stdout)
	name := filepath.Base(filepath.Dir(worktreePath))
	adjective, noun, found := strings.Cut(name, "-")
	require.True(t, found, "generated name %q", name)
	assert.True(t, slices.Contains(worktreeNameAdjectives, adjective), name)
	assert.True(t, slices.Contains(worktreeNameNouns, noun), name)
	testRepository.assertPathPresent(t, testRepository.worktreePath(name))
}

func TestCreateGeneratesDistinctNamesWhenOmittedTwice(t *testing.T) {
	testRepository := newTestRepository(t)

	first := testRepository.runGitWT(t, "create", "--repo", testRepoName)
	require.NoError(t, first.err, first.stderr)
	second := testRepository.runGitWT(t, "create", "--repo", testRepoName)
	require.NoError(t, second.err, second.stderr)

	assert.NotEqual(t, strings.TrimSpace(first.stdout), strings.TrimSpace(second.stdout))
}

func TestFirstUnusedWorktreeNameSkipsTakenNames(t *testing.T) {
	names := []string{"orbital-starship", "reusable-falcon"}
	generate := func() string {
		name := names[0]
		names = names[1:]
		return name
	}
	exists := func(name string) (bool, error) {
		return name == "orbital-starship", nil
	}

	name, err := firstUnusedWorktreeName(generate, exists)
	require.NoError(t, err)
	assert.Equal(t, "reusable-falcon", name)
}

func TestFirstUnusedWorktreeNameReturnsErrorWhenAllNamesTaken(t *testing.T) {
	exists := func(string) (bool, error) {
		return true, nil
	}

	name, err := firstUnusedWorktreeName(func() string { return "orbital-starship" }, exists)
	require.Error(t, err)
	assert.Empty(t, name)
	assert.Contains(t, err.Error(), "unused worktree name")
}

func TestFirstUnusedWorktreeNameReturnsExistsError(t *testing.T) {
	inspectErr := errors.New("stat failed")
	exists := func(string) (bool, error) {
		return false, inspectErr
	}

	name, err := firstUnusedWorktreeName(func() string { return "orbital-starship" }, exists)
	require.ErrorIs(t, err, inspectErr)
	assert.Empty(t, name)
}

func TestRandomWorktreeNameUsesWordLists(t *testing.T) {
	name := randomWorktreeName()
	adjective, noun, found := strings.Cut(name, "-")
	require.True(t, found, name)
	assert.True(t, slices.Contains(worktreeNameAdjectives, adjective), name)
	assert.True(t, slices.Contains(worktreeNameNouns, noun), name)
}

func TestWorktreeNameWordListsAreValid(t *testing.T) {
	wordPattern := regexp.MustCompile(`^[a-z]+$`)
	cases := []struct {
		kind  string
		words []string
	}{
		{kind: "adjectives", words: worktreeNameAdjectives},
		{kind: "nouns", words: worktreeNameNouns},
	}

	seen := make(map[string]string)
	for _, testCase := range cases {
		require.NotEmpty(t, testCase.words, testCase.kind)
		assert.True(t, slices.IsSorted(testCase.words), testCase.kind)
		assert.Equal(t, slices.Compact(slices.Clone(testCase.words)), testCase.words, testCase.kind)
		for _, word := range testCase.words {
			assert.True(t, wordPattern.MatchString(word), word)
			if otherKind, exists := seen[word]; exists {
				t.Errorf("word %q is in both %s and %s", word, otherKind, testCase.kind)
			}
			seen[word] = testCase.kind
		}
	}
}
