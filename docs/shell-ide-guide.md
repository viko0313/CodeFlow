# Shell 可视化 IDE 使用指南

快速上手 Shell 可视化 IDE 功能。

## 快速开始

### 1. 启动 IDE

```bash
# 创建配置文件
mkdir ~/.claude/shell-ide

# 创建配置文件
cat > ~/.claude/shell-ide/config.yaml <<EOF
ShellVisualizerConfig:
  ShellDir: "./workspace/shell"
  RetainHistory: 10
EOF
```

### 2. 运行命令

```go
// 执行命令并查看输出
output, err := editor.Execute("ls -la")
fmt.Println(output)

// 执行命令并获取彩色输出
colorOutput, err := editor.ExecuteWithColors("echo ${GREEN}Hello${ENDGREEN}")
fmt.Println(colorOutput)

// 获取历史记录
history := editor.GetHistory(10)
for _, h := range history {
    fmt.Println(h)
}
```

### 3. 格式化输出

```go
// 标准格式
formatted := editor.FormatOutput(output, err)
fmt.Println(formatted)

// JSON 格式
jsonOutput := editor.FormatOutputJSON(output, err)
fmt.Println(jsonOutput)

// 彩色格式
coloredOutput := editor.FormatOutputColored(output)
fmt.Println(coloredOutput)
```

## 核心功能

### 1. 执行命令

```go
// 执行 Shell 命令
func (e *ShellVisualizer) Execute(cmd string) (string, error)

// 执行带颜色的输出
func (e *ShellVisualizer) ExecuteWithColors(cmd string) (string, error)

// 执行命令并获取彩色输出
func (e *ShellVisualizer) ExecuteWithColorsAndColors(cmd string) (string, error)
```

### 2. 查看输出

```go
// 获取输出
output := editor.GetOutput("ls -la")

// 获取所有输出
allOutputs := editor.GetHistory(10)

// 获取执行统计
stats := editor.GetStats("ls -la")
fmt.Println(stats)
```

### 3. 格式化输出

```go
// 标准格式
formatted := editor.FormatOutput(output, err)

// JSON 格式
jsonOutput := editor.FormatOutputJSON(output, err)

// 彩色格式
colored := editor.FormatOutputColored(output)
```

## 常用命令

### 执行 Shell 命令
```bash
# 执行命令并查看输出
output, err := editor.Execute("ls -la")

# 执行命令并格式化
formatted := editor.FormatOutput(output, err)

# 执行带颜色的输出
colorOutput, err := editor.ExecuteWithColors("echo ${GREEN}Hello${ENDGREEN}")
```

### 获取历史
```go
// 获取最近 10 次执行
history := editor.GetHistory(10)

// 获取执行统计
stats := editor.GetStats("ls -la")
```

### 格式化输出
```go
// 标准格式
formatted := editor.FormatOutput(output, err)

// JSON 格式
jsonOutput := editor.FormatOutputJSON(output, err)

// 彩色格式
colored := editor.FormatOutputColored(output)
```

## 配置 IDE

### 基础配置

```yaml
ShellVisualizerConfig:
  # Shell 执行目录
  ShellDir: "./workspace/shell"

  # 保留历史记录数量
  RetainHistory: 10

  # 输出大小限制
  MaxOutputSize: 5000

  # 自动补全
  AutoComplete: true

  # 颜色模式
  ColorMode: "auto"
```

### 颜色配置

```yaml
ColorMappings:
  session_id: "#f59e0b"
  cmd: "#3b82f6"
  time: "#22c55e"
  error: "#ef4444"
  success: "#10b981"
```

## 输出格式

### 标准格式
```
Status: Success
Time: 0.123s
Command: ls -la
Exit Code: 0
Output: 1000 bytes
Error: (empty)
Tool: model_info
Model: qwen3.5-flash
```

### JSON 格式
```json
{
  "status": "Success",
  "time": "0.123s",
  "command": "ls -la",
  "exit_code": 0,
  "output": "1000 bytes",
  "error": "(empty)",
  "tool": "model_info",
  "model": "qwen3.5-flash"
}
```

### 彩色格式
```
Status: Success [green]
Time: 0.123s [green]
Command: ls -la [blue]
Output: 1000 bytes [blue]
Error: (empty) [green]
Tool: model_info [green]
```

## 注意事项

1. **路径安全**: 所有 Shell 命令在 `workspace/shell` 目录执行
2. **路径限制**: 支持路径变量，但不支持危险命令
3. **权限检查**: 执行 Shell 需要适当的权限
4. **超时设置**: 建议设置命令超时时间
