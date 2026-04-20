# Shell 可视化 IDE 快速指南

## 🎯 核心功能

### 1. 执行 Shell 命令

```go
// 基本执行
output, err := editor.Execute("ls -la")

// 执行并格式化
formatted := editor.FormatOutput(output, err)

// 执行并带颜色输出
colorOutput, err := editor.ExecuteWithColors("echo ${GREEN}Hello${ENDGREEN}")
```

### 2. 查看历史

```go
// 获取最近 10 次执行
history := editor.GetHistory(10)
for _, h := range history {
    fmt.Println(h)
}

// 获取执行统计
stats := editor.GetStats("ls -la")
fmt.Println(stats)
```

### 3. 格式化输出

```go
// 标准格式
sv.FormatOutput(output)

// 彩色格式
sv.FormatOutputColored(output)

// JSON 格式
sv.FormatOutputJSON(output)
```

## 📊 输出样式

### 彩色输出
- **绿色** - 成功
- **红色** - 错误
- **蓝色** - 输出
- **黄色** - 时间
- **橙色** - 命令
- **紫色** - 环境变量

### Shell Output 样式

```go
// 输出
${GREEN}[Success]    # 成功
${RED}[Error]       # 错误
${BLUE}[Output]     # 输出
${BOLD}[Running]    # 运行中

// 命令
${YELLOW}[Command]  # 命令
${BLUE}[Env]        # 环境变量

// 统计
${GRAY}[Time]       # 时间
${GREEN}[Exit]      # 退出码
```

## ⚙️ 配置

### 创建配置

```bash
# 1. 创建目录
mkdir ~/.claude/shell-ide

# 2. 创建配置文件
cat > ~/.claude/shell-ide/config.yaml <<EOF
ShellVisualizerConfig:
  ShellDir: "./workspace/shell"
  RetainHistory: 10
  AutoComplete: true
EOF
```

### 环境变量

```bash
SHELL_VISUALIZER_CONFIG=./shell-ide/config.yaml
SHELL_VISUALIZER_COLOR="auto"
SHELL_VISUALIZER_EDIT="editor"
```

## 🎨 快速参考

### 常用命令

```go
// 执行并查看输出
output, err := editor.Execute("ls -la")

// 执行并格式化
formatted := editor.FormatOutput(output, err)

// 执行并带颜色
colorOutput, err := editor.ExecuteWithColors("echo ${GREEN}Hello${ENDGREEN}")

// 获取历史记录
history := editor.GetHistory(10)

// 格式化输出
colored := editor.FormatOutputColored(output)
```

### API 接口

| 方法 | 功能 |
|------|------|
| `Execute(cmd)` | 执行 Shell 命令 |
| `ExecuteWithColors(cmd)` | 执行并带颜色输出 |
| `ExecuteWithColorsAndColors(cmd)` | 执行并获取颜色 |
| `GetHistory(limit)` | 获取历史记录 |
| `GetStats(cmd)` | 获取执行统计 |
| `FormatOutput(output)` | 格式化标准输出 |
| `FormatOutputJSON(output)` | 格式化 JSON 输出 |
| `FormatOutputColored(output)` | 格式化彩色输出 |

## 🚀 快速上手

```go
// 1. 启动 IDE（创建配置）
mkdir ~/.claude/shell-ide
cat > ~/.claude/shell-ide/config.yaml <<EOF
ShellVisualizerConfig:
  ShellDir: "./workspace/shell"
  RetainHistory: 10
EOF

// 2. 执行命令
output, err := editor.Execute("ls -la")
if err != nil {
    fmt.Println(err)
}

// 3. 格式化输出
formatted := editor.FormatOutput(output, err)
fmt.Println(formatted)
```

## 📝 颜色映射

```go
const (
    Success  string = "#10b981"  // 绿色
    Error    string = "#ef4444"   // 红色
    Output   string = "#3b82f6"   // 蓝色
    Time     string = "#22c55e"   // 绿色
    Command  string = "#8b5cf6"   // 紫色
    Env      string = "#f59e0b"   // 橙色
)
```

## ⚡ 快速测试

```bash
# 1. 启动 IDE
go run .

# 2. 执行命令
cd workspace/shell
cat > test.sh <<'EOF'
${GREEN}echo Hello${ENDGREEN}
${RED}echo Error${ENDRED}
EOF

# 3. 执行 Shell 命令
cmd, err := editor.Execute("cat test.sh")
if err != nil {
    fmt.Println(err)
}

// 4. 获取输出
formatted := editor.FormatOutput(cmd, nil)
fmt.Println(formatted)
```

## 🎯 下一步

- 查看 [shell-ide-guide.md](./shell-ide-guide.md) 详细文档
- 查看 [shell-visualizer.md](./shell-visualizer.md) 可视化原理
- 查看 [shell-ide-config.example.yaml](./shell-ide-config.example.yaml) 配置文件
