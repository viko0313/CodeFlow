package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type UserProfileTool struct {
	memoryDir string
}

func NewReadUserProfileTool(memoryDir string) *UserProfileTool {
	return &UserProfileTool{memoryDir: memoryDir}
}

func NewSaveUserProfileTool(memoryDir string) *UserProfileTool {
	return &UserProfileTool{memoryDir: memoryDir}
}

func (t *UserProfileTool) profilePath() string {
	return filepath.Join(t.memoryDir, "user_profile.md")
}

type ReadUserProfileTool struct {
	*UserProfileTool
}

func NewReadProfileTool(memoryDir string) *ReadUserProfileTool {
	return &ReadUserProfileTool{UserProfileTool: &UserProfileTool{memoryDir: memoryDir}}
}

func (t *ReadUserProfileTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "read_user_profile",
		Desc: "Read the long-term user profile Markdown memory.",
	}, nil
}

func (t *ReadUserProfileTool) InvokableRun(ctx context.Context, argsInJSON string, opts ...tool.Option) (string, error) {
	data, err := os.ReadFile(t.profilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return "No user profile saved yet.", nil
		}
		return "", err
	}
	if len(data) == 0 {
		return "No user profile saved yet.", nil
	}
	return string(data), nil
}

type SaveUserProfileTool struct {
	*UserProfileTool
}

func NewSaveProfileTool(memoryDir string) *SaveUserProfileTool {
	return &SaveUserProfileTool{UserProfileTool: &UserProfileTool{memoryDir: memoryDir}}
}

func (t *SaveUserProfileTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "save_user_profile",
		Desc: "Overwrite the long-term user profile Markdown memory. Read the existing profile first and pass the complete updated profile.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"new_content": {
				Type:     schema.String,
				Desc:     "Complete updated Markdown content for the user profile",
				Required: true,
			},
		}),
	}, nil
}

func (t *SaveUserProfileTool) InvokableRun(ctx context.Context, argsInJSON string, opts ...tool.Option) (string, error) {
	var args struct {
		NewContent string `json:"new_content"`
	}
	if err := json.Unmarshal([]byte(argsInJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if err := os.MkdirAll(t.memoryDir, 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(t.profilePath(), []byte(args.NewContent), 0600); err != nil {
		return "", err
	}
	return "User profile updated successfully.", nil
}
