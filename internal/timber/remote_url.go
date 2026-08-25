package timber

import (
	"fmt"
	"net/url"
	"path"
	"strings"
)

// resolveRemoteURL maps user input to a git remote URL.
//
// Schema-less relative paths (e.g. "nnutter/timber") become
// https://github.com/<path>. Absolute URLs, SSH forms, and local paths pass
// through unchanged.
func resolveRemoteURL(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", fmt.Errorf("repository URL is required")
	}

	switch {
	case strings.Contains(trimmed, "://"):
		return trimmed, nil
	case strings.HasPrefix(trimmed, "git@"):
		return trimmed, nil
	case strings.HasPrefix(trimmed, "github.com:"):
		return "git@" + trimmed, nil
	case strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "."):
		return trimmed, nil
	case strings.HasPrefix(trimmed, "~"):
		return trimmed, nil
	case looksLikeSSHShorthand(trimmed):
		return trimmed, nil
	default:
		return "https://github.com/" + strings.TrimPrefix(trimmed, "/"), nil
	}
}

func looksLikeSSHShorthand(input string) bool {
	// host:path without scheme, e.g. gitlab.com:group/repo.git
	if strings.Contains(input, "://") {
		return false
	}
	host, repoPath, found := strings.Cut(input, ":")
	if !found || host == "" || repoPath == "" {
		return false
	}
	return !strings.Contains(host, "/") && strings.Contains(host, ".")
}

func defaultRepoNameFromRemote(remoteURL string) (string, error) {
	name := remoteURL
	if strings.HasPrefix(name, "git@") {
		_, remainder, found := strings.Cut(name, ":")
		if found {
			name = remainder
		}
	} else if parsed, err := url.Parse(name); err == nil && parsed.Path != "" {
		name = parsed.Path
	}

	name = strings.TrimSuffix(name, "/")
	name = path.Base(name)
	name = strings.TrimSuffix(name, bareRepoSuffix)
	if name == "" || name == "." || name == "/" {
		return "", fmt.Errorf("could not derive repository name from %q", remoteURL)
	}
	return name, nil
}
