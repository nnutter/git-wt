package gitwt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	agentTabLabel  = "Agent"
	editorTabLabel = "Editor"
	shellTabLabel  = "Shell"
)

type herdrResource struct {
	WorkspaceID string `json:"workspace_id"`
	TabID       string `json:"tab_id"`
	PaneID      string `json:"pane_id"`
}

type herdrWorkspaceCreateResponse struct {
	Result struct {
		Workspace herdrResource `json:"workspace"`
		Tab       herdrResource `json:"tab"`
		RootPane  herdrResource `json:"root_pane"`
	} `json:"result"`
}

type herdrTabCreateResponse struct {
	Result struct {
		Tab      herdrResource `json:"tab"`
		RootPane herdrResource `json:"root_pane"`
	} `json:"result"`
}

type herdrSpace struct {
	workspaceID  string
	worktreePath string
	agentTabID   string
	agentPaneID  string
	editorPaneID string
}

func runningInHerdr() bool {
	return os.Getenv("HERDR_ENV") == "1"
}

func openHerdrSpace(ctx context.Context, worktree managedWorktree) (returnErr error) {
	space, returnErr := createHerdrSpace(ctx, worktree)
	if space.workspaceID == "" {
		return returnErr
	}

	defer func() {
		if returnErr != nil {
			returnErr = errors.Join(returnErr, space.close(context.WithoutCancel(ctx)))
		}
	}()

	if returnErr != nil {
		return returnErr
	}
	if returnErr = space.configure(ctx); returnErr != nil {
		return returnErr
	}
	return space.focus(ctx)
}

func createHerdrSpace(ctx context.Context, worktree managedWorktree) (herdrSpace, error) {
	absolutePath, err := filepath.Abs(worktree.Path)
	if err != nil {
		return herdrSpace{}, fmt.Errorf("resolve worktree path for herdr: %w", err)
	}

	output, err := runHerdr(
		ctx,
		"workspace", "create", "--cwd", absolutePath,
		"--label", worktree.Repo, "--no-focus",
	)
	if err != nil {
		return herdrSpace{}, err
	}

	return parseHerdrSpace(output, absolutePath)
}

func parseHerdrSpace(output []byte, worktreePath string) (herdrSpace, error) {
	var response herdrWorkspaceCreateResponse
	if err := json.Unmarshal(output, &response); err != nil {
		return herdrSpace{}, fmt.Errorf("decode herdr workspace create response: %w", err)
	}

	space := herdrSpace{
		workspaceID:  response.Result.Workspace.WorkspaceID,
		worktreePath: worktreePath,
		agentTabID:   response.Result.Tab.TabID,
		agentPaneID:  response.Result.RootPane.PaneID,
	}
	return space, space.validateInitialResources()
}

func (x herdrSpace) validateInitialResources() error {
	if x.workspaceID == "" {
		return errors.New("herdr workspace create response has no workspace ID")
	}
	if x.agentTabID == "" {
		return errors.New("herdr workspace create response has no tab ID")
	}
	if x.agentPaneID == "" {
		return errors.New("herdr workspace create response has no root pane ID")
	}
	return nil
}

func (x *herdrSpace) configure(ctx context.Context) error {
	if _, err := runHerdr(ctx, "tab", "rename", x.agentTabID, agentTabLabel); err != nil {
		return err
	}

	editorPaneID, err := x.createTab(ctx, editorTabLabel)
	if err != nil {
		return err
	}
	x.editorPaneID = editorPaneID

	if _, err := x.createTab(ctx, shellTabLabel); err != nil {
		return err
	}
	return x.startCommands(ctx)
}

func (x herdrSpace) createTab(ctx context.Context, label string) (string, error) {
	output, err := runHerdr(
		ctx,
		"tab", "create", "--workspace", x.workspaceID, "--cwd", x.worktreePath,
		"--label", label, "--no-focus",
	)
	if err != nil {
		return "", err
	}
	return parseHerdrTabPaneID(output)
}

func parseHerdrTabPaneID(output []byte) (string, error) {
	var response herdrTabCreateResponse
	if err := json.Unmarshal(output, &response); err != nil {
		return "", fmt.Errorf("decode herdr tab create response: %w", err)
	}
	if response.Result.Tab.TabID == "" || response.Result.RootPane.PaneID == "" {
		return "", errors.New("herdr tab create response has incomplete tab resources")
	}

	return response.Result.RootPane.PaneID, nil
}

func (x herdrSpace) startCommands(ctx context.Context) error {
	if _, err := runHerdr(ctx, "pane", "run", x.agentPaneID, "pi"); err != nil {
		return err
	}
	if _, err := runHerdr(ctx, "pane", "run", x.editorPaneID, "nvim ."); err != nil {
		return err
	}
	return nil
}

func (x herdrSpace) focus(ctx context.Context) error {
	if _, err := runHerdr(ctx, "workspace", "focus", x.workspaceID); err != nil {
		return err
	}
	_, err := runHerdr(ctx, "tab", "focus", x.agentTabID)
	return err
}

func (x herdrSpace) close(ctx context.Context) error {
	_, err := runHerdr(ctx, "workspace", "close", x.workspaceID)
	return err
}

func runHerdr(ctx context.Context, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "herdr", args...)
	output, err := command.CombinedOutput()
	if err == nil {
		return output, nil
	}

	operation := strings.Join(args[:min(2, len(args))], " ")
	message := strings.TrimSpace(string(output))
	if message == "" {
		return nil, fmt.Errorf("herdr %s: %w", operation, err)
	}
	return nil, fmt.Errorf("herdr %s: %w: %s", operation, err, message)
}
