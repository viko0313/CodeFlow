# Shell 可视化

## 概述

Shell 可视化提供实时 Shell 命令执行和输出查看功能，支持彩色输出、执行状态追踪和历史记录。

## 文件结构

```
internal/tools/
├── shell_visualizer.go    # 主实现
├── shell_visualizer_test.go  # 测试文件
└── config.go               # 配置模块
```

## 功能特性

### 1. **实时输出显示**
- 自动显示 Shell 命令执行结果
- 支持彩色输出（按优先级显示）
- 显示退出码（非零表示错误）

### 2. **历史记录管理**
- 自动记录最近 10 次执行
- 支持清理过期历史（maxAge 配置）
- 可通过 API 访问历史

### 3. **执行统计**
- 命令执行时间
- 总执行次数
- 平均执行速度

### 4. **彩色输出**
支持 10 种颜色：
- `session_id` - 会话标识
- `cmd` - 命令标识
- `time` - 时间标识
- `error` - 错误
- `success` - 成功
- `env` - 环境变量
- `data` - 数据
- `err` - 错误信息
- `out` - 标准输出
- `err2` - 标准错误

## 配置

创建配置文件 `ShellVisualizerConfig.yaml`:

```yaml
ShellVisualizerConfig:
  ShellDir: "./workspace/shell"
  RetainHistory: 10
  MaxOutputSize: 5000
```

## API 接口

### 执行 Shell
```go
// 执行单个命令
func (sv *ShellVisualizer) ExecuteShell(command string) (string, error)

// 获取最新输出
func (sv *ShellVisualizer) LatestOutput() ShellOutput

// 获取所有输出
func (sv *ShellVisualizer) LatestOutputs() []ShellOutput

// 获取输出计数
func (sv *ShellVisualizer) OutputCount() int
```

### 格式化输出

**标准格式**:
```go
sv.FormatOutput(output)
```

**彩色格式**:
```go
sv.FormatOutputColored(output)
```

**JSON 格式**:
```go
sv.FormatOutputJSON(output)
```

## 颜色映射

颜色优先级：`success > error > data > cmd > env > time > err > out > err2`

## 性能优化

- 使用 `sync.RWMutex` 保证读写互斥
- 延迟输出接收 (`go receiveOutputs`)
- 历史输出缓存 (10 条)
- 输出大小限制 (5KB)
