package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type DynamicSkillTool struct {
	name        string
	desc        string
	folderName  string
	content     string
	officeShell *ExecuteShellTool
}

func LoadDynamicSkillTools(skillsDir, officeDir string) ([]tool.BaseTool, error) {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var loaded []tool.BaseTool
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		folder := filepath.Join(skillsDir, entry.Name())
		docPath := filepath.Join(folder, "SKILL.md")
		if _, err := os.Stat(docPath); err != nil {
			docPath = filepath.Join(folder, "README.md")
		}
		data, err := os.ReadFile(docPath)
		if err != nil {
			continue
		}
		content := string(data)
		name := sanitizeToolName(firstMetadataValue(content, "name", entry.Name()))
		desc := firstMetadataValue(content, "description", "External CyberClaw skill "+entry.Name())
		loaded = append(loaded, &DynamicSkillTool{
			name:        name,
			desc:        desc,
			folderName:  entry.Name(),
			content:     content,
			officeShell: NewExecuteShellTool(officeDir),
		})
	}
	return loaded, nil
}

func (t *DynamicSkillTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: t.name,
		Desc: t.desc + "\nThis is an external skill. First call with mode=help, then use mode=run with a command containing {baseDir}.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"mode":    {Type: schema.String, Desc: "help or run", Required: true},
			"command": {Type: schema.String, Desc: "Command for run mode. Use {baseDir} as the skill directory."},
		}),
	}, nil
}

func (t *DynamicSkillTool) InvokableRun(ctx context.Context, argsInJSON string, opts ...tool.Option) (string, error) {
	var args struct {
		Mode    string `json:"mode"`
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(argsInJSON), &args); err != nil {
		return "", err
	}
	switch args.Mode {
	case "help":
		if len(t.content) > 3000 {
			return t.content[:3000] + "\n\n...[skill help truncated]...", nil
		}
		return t.content, nil
	case "run":
		if strings.TrimSpace(args.Command) == "" {
			return "", fmt.Errorf("command is required in run mode")
		}
		command := strings.ReplaceAll(args.Command, "{baseDir}", filepath.ToSlash(filepath.Join("skills", t.folderName)))
		payload, _ := json.Marshal(map[string]string{"command": command})
		return t.officeShell.InvokableRun(ctx, string(payload))
	default:
		return "", fmt.Errorf("mode must be help or run")
	}
}

func firstMetadataValue(content, key, fallback string) string {
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `:\s*(.+)$`)
	match := re.FindStringSubmatch(content)
	if len(match) < 2 {
		return fallback
	}
	return strings.Trim(strings.TrimSpace(match[1]), `"'`)
}

func sanitizeToolName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "dynamic_skill"
	}
	re := regexp.MustCompile(`[^a-zA-Z0-9_-]+`)
	return re.ReplaceAllString(name, "_")
}
