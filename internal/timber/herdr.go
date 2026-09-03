package timber

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"strings"
)

const (
	agentTabLabel = "Agent"
	shellTabLabel = "Shell"
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

type herdrPaneCurrentResponse struct {
	Result struct {
		Pane herdrResource `json:"pane"`
	} `json:"result"`
}

type herdrSpace struct {
	runtime        Runtime
	workspaceID    string
	worktreePath   string
	agentTabID     string
	agentPaneID    string
	agentPaneLabel string
}

func (x Runtime) openHerdrSpace(ctx context.Context, worktree managedWorktree) (returnErr error) {
	space, returnErr := x.createHerdrSpace(ctx, worktree)
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

func (x Runtime) defineCurrentHerdrSpace(ctx context.Context, worktree managedWorktree) error {
	space, err := x.currentHerdrSpace(ctx, worktree)
	if err != nil {
		return err
	}
	if err := space.configure(ctx); err != nil {
		return err
	}
	return space.focus(ctx)
}

func (x Runtime) createHerdrSpace(ctx context.Context, worktree managedWorktree) (herdrSpace, error) {
	absolutePath, err := x.herdrWorktreePath(worktree)
	if err != nil {
		return herdrSpace{}, err
	}

	output, err := x.runHerdr(
		ctx,
		"workspace", "create", "--cwd", absolutePath,
		"--label", worktree.Repo, "--no-focus",
	)
	if err != nil {
		return herdrSpace{}, err
	}

	return parseHerdrSpace(x, output, absolutePath, worktree.Name)
}

func (x Runtime) currentHerdrSpace(ctx context.Context, worktree managedWorktree) (herdrSpace, error) {
	absolutePath, err := x.herdrWorktreePath(worktree)
	if err != nil {
		return herdrSpace{}, err
	}

	output, err := x.runHerdr(ctx, "pane", "current", "--current")
	if err != nil {
		return herdrSpace{}, err
	}
	return parseCurrentHerdrSpace(x, output, absolutePath, worktree.Name)
}

func (x Runtime) herdrWorktreePath(worktree managedWorktree) (string, error) {
	absolutePath, err := x.absolutePath(worktree.Path)
	if err != nil {
		return "", fmt.Errorf("resolve worktree path for herdr: %w", err)
	}
	return absolutePath, nil
}

func parseHerdrSpace(runtime Runtime, output []byte, worktreePath string, worktreeName string) (herdrSpace, error) {
	var response herdrWorkspaceCreateResponse
	if err := json.Unmarshal(output, &response); err != nil {
		return herdrSpace{}, fmt.Errorf("decode herdr workspace create response: %w", err)
	}

	space := herdrSpace{
		runtime:        runtime,
		workspaceID:    response.Result.Workspace.WorkspaceID,
		worktreePath:   worktreePath,
		agentTabID:     response.Result.Tab.TabID,
		agentPaneID:    response.Result.RootPane.PaneID,
		agentPaneLabel: worktreeName,
	}
	return space, space.validateInitialResources()
}

func parseCurrentHerdrSpace(runtime Runtime, output []byte, worktreePath string, worktreeName string) (herdrSpace, error) {
	var response herdrPaneCurrentResponse
	if err := json.Unmarshal(output, &response); err != nil {
		return herdrSpace{}, fmt.Errorf("decode herdr pane current response: %w", err)
	}

	space := herdrSpace{
		runtime:        runtime,
		workspaceID:    response.Result.Pane.WorkspaceID,
		worktreePath:   worktreePath,
		agentTabID:     response.Result.Pane.TabID,
		agentPaneID:    response.Result.Pane.PaneID,
		agentPaneLabel: worktreeName,
	}
	if space.workspaceID == "" || space.agentTabID == "" || space.agentPaneID == "" {
		return herdrSpace{}, errors.New("herdr pane current response has incomplete pane resources")
	}
	return space, nil
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

func (x herdrSpace) configure(ctx context.Context) error {
	if _, err := x.runtime.runHerdr(ctx, "tab", "rename", x.agentTabID, agentTabLabel); err != nil {
		return err
	}
	if _, err := x.runtime.runHerdr(ctx, "pane", "rename", x.agentPaneID, x.agentPaneLabel); err != nil {
		return err
	}
	if err := x.createTab(ctx, shellTabLabel); err != nil {
		return err
	}
	return x.startCommands(ctx)
}

func (x herdrSpace) createTab(ctx context.Context, label string) error {
	output, err := x.runtime.runHerdr(
		ctx,
		"tab", "create", "--workspace", x.workspaceID, "--cwd", x.worktreePath,
		"--label", label, "--no-focus",
	)
	if err != nil {
		return err
	}
	return parseHerdrTabCreateResponse(output)
}

func parseHerdrTabCreateResponse(output []byte) error {
	var response herdrTabCreateResponse
	if err := json.Unmarshal(output, &response); err != nil {
		return fmt.Errorf("decode herdr tab create response: %w", err)
	}
	if response.Result.Tab.TabID == "" || response.Result.RootPane.PaneID == "" {
		return errors.New("herdr tab create response has incomplete tab resources")
	}
	return nil
}

func (x herdrSpace) startCommands(ctx context.Context) error {
	_, err := x.runtime.runHerdr(ctx, "pane", "run", x.agentPaneID, "pi")
	return err
}

func (x herdrSpace) focus(ctx context.Context) error {
	if _, err := x.runtime.runHerdr(ctx, "workspace", "focus", x.workspaceID); err != nil {
		return err
	}
	_, err := x.runtime.runHerdr(ctx, "tab", "focus", x.agentTabID)
	return err
}

func (x herdrSpace) close(ctx context.Context) error {
	_, err := x.runtime.runHerdr(ctx, "workspace", "close", x.workspaceID)
	return err
}

func (x Runtime) runHerdr(ctx context.Context, args ...string) ([]byte, error) {
	command := x.command(ctx, "herdr", args...)
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
