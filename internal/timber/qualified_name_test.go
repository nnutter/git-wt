package timber

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseQualifiedName(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	runtime := testRepository.runtime

	qualified, err := runtime.parseQualifiedName("feature/login")
	require.NoError(t, err)
	assert.Equal(t, qualifiedName{Name: "feature/login"}, qualified)

	qualified, err = runtime.parseQualifiedName(at(testRepoName, "feature/login"))
	require.NoError(t, err)
	assert.Equal(t, qualifiedName{Name: "feature/login", Repo: testRepoName}, qualified)

	qualified, err = runtime.parseQualifiedName(at(testRepoName, ""))
	require.NoError(t, err)
	assert.Equal(t, qualifiedName{Repo: testRepoName}, qualified)

	_, err = runtime.parseQualifiedName("feature/login@")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing repository")

	_, err = runtime.parseQualifiedName("feature/login@unknown")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown repository")
}

func TestParseRepoOnlyArg(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	runtime := testRepository.runtime

	repo, err := runtime.parseRepoOnlyArg(at(testRepoName, ""))
	require.NoError(t, err)
	assert.Equal(t, testRepoName, repo)

	_, err = runtime.parseRepoOnlyArg(at(testRepoName, "feature/login"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected @<repo>")

	_, err = runtime.parseRepoOnlyArg(testRepoName)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected @<repo>")
}

func TestRejectAtInWorktreeName(t *testing.T) {
	t.Parallel()
	require.NoError(t, rejectAtInWorktreeName("feature/login"))
	require.NoError(t, rejectAtInWorktreeName(""))
	err := rejectAtInWorktreeName("foo@bar")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not contain @")
}
