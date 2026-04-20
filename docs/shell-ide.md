# Shell 可视化 IDE

类似 Claude Code 的 Shell 代码编辑器，提供交互式 Shell 命令执行界面。

## 功能特性

### 🎯 交互式编辑
```go
// 编辑 Shell 命令
cmd, err := editor.Execute(cmd)
if err != nil {
    fmt.Println(err)
    return
}
```

### 🎨 彩色输出
- Success: 绿色
- Error: 红色
- Output: 蓝色
- Time: 灰色
- Command: 黄色

### 📁 历史记录
- 自动保存最近 10 次执行
- 支持清理过期历史 (15ms)
- 查看完整执行时间

### ⚡ 快速执行
- 支持变量替换
- 自动检测错误
- 实时输出显示

## 使用方法

### 运行命令
```go
// 直接执行
output, err := editor.Execute("ls -la")
fmt.Println(output)

// 执行带颜色的输出
colorOutput, err := editor.ExecuteWithColors("echo ${GREEN}Hello${ENDGREEN}")
fmt.Println(colorOutput)
```

### 自动补全
```go
// 自动补全命令
completion, err := editor.Complete([]string{"ls", "-la"})
if err != nil {
    fmt.Println(err)
}
```

## 颜色样式

### 标准输出
```go
${GREEN}Hello World${ENDGREEN}        # 绿色文本
${BOLD}Running ${YELLOW}process${ENDBOLD}  # 加粗和颜色混合
```

### 错误输出
```go
${RED}Command failed: ${RED}bash: :${ENDRED}
${BOLD}Error: ${RED}exit code ${RED}1${ENDBOLD}
```

### 状态标签
```go
${GREEN}[Success]     # 成功
${RED}[Error]       # 错误
${BOLD}[Running]    # 运行中
${BLUE}[Output]     # 输出
${YELLOW}[Time]     # 时间
```

## 代码编辑

```go
// 编辑 Shell 命令
var result *ShellOutput

cmd, err := editor.Execute("ls -la")
if err != nil {
    fmt.Println(err)
}

// 格式化输出
formatted := editor.FormatOutput(result, err)
fmt.Println(formatted)

// 格式化 JSON
jsonOutput := editor.FormatOutputJSON(result, err)
fmt.Println(jsonOutput)
```

### 支持的操作

| 操作 | 描述 | 示例 |
|------|------|------|
| `Execute` | 执行命令 | `editor.Execute("ls")` |
| `ExecuteWithColors` | 带颜色的输出 | `editor.ExecuteWithColors("echo ${GREEN}Hello${ENDGREEN}")` |
| `Complete` | 自动补全命令 | `editor.Complete([]string{"ls"})` |
| `GetHistory` | 获取历史记录 | `editor.GetHistory(10)` |
| `ClearHistory` | 清理历史 | `editor.ClearHistory()` |
| `FormatOutput` | 格式化输出 | `editor.FormatOutput(output, err)` |
| `FormatOutputJSON` | JSON 格式输出 | `editor.FormatOutputJSON(output, err)` |

## 配置

### 创建配置文件

```bash
mkdir ~/.claude/shell-ide
```

```yaml
# shell-ide/config.yaml
ShellVisualizerConfig:
  ShellDir: "./workspace/shell"
  RetainHistory: 10
  MaxOutputSize: 5000
  ColorMode: "auto"
  AutoComplete: true
```

### 环境变量配置

```bash
# .env
SHELL_VISUALIZER_CONFIG=./shell-ide/config.yaml
SHELL_VISUALIZER_COLOR="auto"
SHELL_VISUALIZER_EDIT="editor"
```

## API 接口

### Shell 编辑器

```go
// 执行命令
func (e *ShellVisualizer) Execute(cmd string) (string, error)

// 执行命令并格式化
func (e *ShellVisualizer) ExecuteWithColors(cmd string) (string, error)

// 执行命令并获取彩色输出
func (e *ShellVisualizer) ExecuteWithColorsAndColors(cmd string) (string, error)

// 获取命令
func (e *ShellVisualizer) GetCommand(cmd string) (string, error)

// 获取输出
func (e *ShellVisualizer) GetOutput(cmd string) (string, error)

// 获取退出码
func (e *ShellVisualizer) GetExitCode(cmd string) (int, error)

// 获取工具信息
func (e *ShellVisualizer) GetToolInfo(name string) *tool.ToolInfo

// 获取历史记录
func (e *ShellVisualizer) GetHistory(limit int) []ShellOutput

// 清理历史
func (e *ShellVisualizer) ClearHistory(maxAge time.Duration)
```

### ShellVisualizer 工具

```go
// 执行命令
func (v *ShellVisualizer) ExecuteShell(cmd string) (string, error)

// 执行命令并格式化
func (v *ShellVisualizer) ExecuteCommand(cmd string) (string, error)

// 执行标准输出
func (v *ShellVisualizer) ExecuteStdout(cmd string) (string, error)

// 执行错误输出
func (v *ShellVisualizer) ExecuteStderr(cmd string) (string, error)

// 执行 Shell
func (v *ShellVisualizer) ExecuteShell(cmd string) (string, error)

// 获取输出
func (v *ShellVisualizer) LatestOutput() ShellOutput

// 获取所有输出
func (v *ShellVisualizer) LatestOutputs() []ShellOutput

// 获取输出计数
func (v *ShellVisualizer) OutputCount() int
```

### ShellOutput 结构

```go
type ShellOutput struct {
    ExecutionID string `json:"execution_id"`
    ExecutionTime time.Time  `json:"execution_time"`
    Command     string `json:"command"`
    CommandOutput []byte `json:"command_output"`
    ExitCode    int    `json:"exit_code"`
    Stdout      string `json:"stdout"`
    Stderr      string `json:"stderr"`
    StartTime   time.Time `json:"start_time"`
    ElapsedTime time.Duration  `json:"elapsed_time"`
    ShellCommand string  `json:"shell_command"`
    Environment []string  `json:"environment"`
    Timestamp   time.Time `json:"timestamp"`
}
```

## 样式配置

### 基础样式
```go
FormatOutput(output ShellOutput) string
FormatOutputJSON(output ShellOutput) string
FormatOutputColored(output ShellOutput) string
```

### 配置样式
```go
// 颜色映射
const (
    ColorSuccess  string = "#10b981"
    ColorError    string = "#ef4444"
    ColorOutput   string = "#3b82f6"
    ColorTime     string = "#22c55e"
    ColorCmd      string = "#8b5cf6"
    ColorEnv      string = "#f59e0b"
)
```

### 样式选择
```go
// 标准格式
sv.FormatOutput(output)

// 彩色格式
sv.FormatOutputColored(output)

// JSON 格式
sv.FormatOutputJSON(output)
```

## 测试

```bash
# 运行测试
go test ./...

# 测试 Shell 执行
go test ./internal/tools -run TestShell

# 测试 IDE 功能
go test ./internal/tools -run TestShellIDE
```

## 注意事项

1. **路径限制**: 所有 Shell 命令执行在 `workspace/shell` 目录内
2. **路径安全**: 支持路径变量，但不支持危险命令
3. **权限检查**: 执行 Shell 需要适当的权限
4. **超时设置**: 建议设置命令超时时间
