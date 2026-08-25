package timber

import (
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

func listRegisteredRepos() ([]registeredRepo, error) {
	directory := reposDirectory()
	entries, err := os.ReadDir(directory)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read repos directory %q: %w", directory, err)
	}

	repos := make([]registeredRepo, 0)
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, bareRepoSuffix) {
			continue
		}
		repoName := strings.TrimSuffix(name, bareRepoSuffix)
		if repoName == "" {
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

func registeredRepoByName(name string) (registeredRepo, error) {
	repos, err := listRegisteredRepos()
	if err != nil {
		return registeredRepo{}, err
	}
	for _, repo := range repos {
		if repo.Name == name {
			return repo, nil
		}
	}
	return registeredRepo{}, fmt.Errorf("unknown repository %q", name)
}

func openRegisteredRepository(name string) (*Repository, registeredRepo, error) {
	repo, err := registeredRepoByName(name)
	if err != nil {
		return nil, registeredRepo{}, err
	}
	repository, err := openBareRepository(repo.BarePath)
	if err != nil {
		return nil, registeredRepo{}, err
	}
	return repository, repo, nil
}
