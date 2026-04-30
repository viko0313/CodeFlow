package storage

import "github.com/viko0313/CodeFlow/internal/codeflow/workspace"

type WorkspaceStore interface {
	Register(rootPath, name string, metadata map[string]string) (*workspace.Workspace, error)
	ListWorkspaces() ([]workspace.Workspace, error)
	GetWorkspaceByID(id string) (*workspace.Workspace, error)
	GetWorkspaceByRoot(rootPath string) (*workspace.Workspace, error)
	GetCurrentWorkspace() (*workspace.Workspace, error)
	SwitchWorkspace(id string) (*workspace.Workspace, error)
	RemoveWorkspace(id string) error
}
