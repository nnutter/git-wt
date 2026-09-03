package timber

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type registeredRepo struct {
	Name     string
	BarePath string
}

func (x Runtime) listRegisteredRepos() ([]registeredRepo, error) {
	directory := x.reposDirectory()
	entries, err := os.ReadDir(directory)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read repos directory %q: %w", directory, err)
	}

	repos := make([]registeredRepo, 0)
	for _, entry := range entries {
		name := entry.Name()
		repoName, found := strings.CutSuffix(name, bareRepoSuffix)
		if !found || repoName == "" {
			continue
		}

		fullPath := filepath.Join(directory, name)
		info, err := os.Stat(fullPath)
		if err != nil {
			return nil, fmt.Errorf("stat registered repo %q: %w", name, err)
		}
		if !info.IsDir() {
			continue
		}

		repos = append(repos, registeredRepo{
			Name:     repoName,
			BarePath: fullPath,
		})
	}

	slices.SortFunc(repos, func(left, right registeredRepo) int {
		return strings.Compare(left.Name, right.Name)
	})
	return repos, nil
}

func (x Runtime) registeredRepoByName(name string) (registeredRepo, error) {
	repos, err := x.listRegisteredRepos()
	if err != nil {
		return registeredRepo{}, err
	}
	index := slices.IndexFunc(repos, func(repo registeredRepo) bool {
		return repo.Name == name
	})
	if index >= 0 {
		return repos[index], nil
	}
	return registeredRepo{}, fmt.Errorf("unknown repository %q", name)
}

func (x Runtime) openRegisteredRepository(name string) (*Repository, registeredRepo, error) {
	repo, err := x.registeredRepoByName(name)
	if err != nil {
		return nil, registeredRepo{}, err
	}
	repository, err := openBareRepository(x, repo.BarePath)
	if err != nil {
		return nil, registeredRepo{}, err
	}
	return repository, repo, nil
}
