# CyberClaw-Go

CyberClaw-Go 是 CyberClaw 的 Go 版本实现，核心目标是把智能体的“想什么、调什么工具、消耗多少 token、是否触碰沙盒边界”尽量透明地展示出来。它基于 CloudWeGo Eino 组件构建 ReAct 工具调用循环，支持云端模型和本地 Ollama 模型，并把文件、Shell、记忆、任务提醒都限制在可审计的工作区内。

> 这份 README 是 Go 版说明文档，内容按当前代码实现重新组织，不复刻 Python 版文案。

## 当前能力

- 多模型适配：Ark/火山、Qwen/DashScope、OpenAI 兼容接口、本地 Ollama。
- ReAct 工具循环：模型可调用计算器、时间、模型信息、记忆、任务、沙盒文件和 Shell 工具。
- 安全沙盒：所有文件与 Shell 操作默认限制在 `workspace/office`，拦截 `..`、绝对路径、盘符路径和危险 shell 跳转。
- 记忆系统：长期用户画像保存在 `workspace/memory/user_profile.md`，近期对话按 JSONL 记录。
- 心跳任务：任务保存在 `workspace/tasks.json`，到期后在 REPL 中输出提醒。
- 运行可视化：每轮对话结束后显示模型、token、耗时、模块流转、工具、记忆和沙盒状态。
- 审计日志：JSONL 日志写入 `workspace/logs/audit_YYYY-MM-DD.json`。

## 项目结构

```text
cmd/agent/main.go          REPL 入口与本地配置向导
internal/agent/            ReAct 循环、上下文、token 与运行统计
internal/config/           YAML/env/.env/config.local.yaml 配置加载
internal/model/            多模型工厂
internal/tools/            内置工具、沙盒工具、任务、技能、记忆工具
internal/heartbeat/        到期任务检查
internal/logger/           审计日志
workspace/                 本地运行数据，不建议提交
```

## 快速开始

```powershell
cd e:\agent\CyberClaw-Go
go run .\cmd\agent
```

也可以先编译：

```powershell
go build -o cyberclaw.exe .\cmd\agent
.\cyberclaw.exe
```

退出 REPL：

```text
exit
```

## 配置优先级

配置按以下顺序覆盖，越靠前优先级越高：

```text
环境变量 > config.local.yaml > config.yaml > 内置默认值
```

敏感信息不要写入 `config.yaml`。推荐方式是：

- API key 放在 `.env` 或系统环境变量。
- `config.local.yaml` 只写 `${QWEN_API_KEY}`、`${ARK_API_KEY}` 这类引用。
- `.env`、`config.local.yaml`、`workspace/` 已在 `.gitignore` 中忽略。

交互式配置：

```powershell
go run .\cmd\agent config
```

## 云端 Qwen 示例

`.env`：

```env
QWEN_API_KEY=sk-your-key
```

`config.local.yaml`：

```yaml
provider: "qwen"
model: "qwen3.5-flash"
api_key: ${QWEN_API_KEY}
base_url: "https://dashscope.aliyuncs.com/compatible-mode/v1"
temperature: 0.00
workspace: "./workspace"
max_memory: 100
max_turns: 50
max_actions: 20
```

## 本地 Ollama Qwen 示例

先确认 Ollama 服务可用：

```powershell
Invoke-RestMethod http://localhost:11434/api/tags
```

当前验证过的本地模型名：

```text
qwen3.5:4b
```

`config.local.yaml`：

```yaml
provider: "ollama"
model: "qwen3.5:4b"
api_key: ""
base_url: "http://localhost:11434"
temperature: 0.00
workspace: "./workspace"
max_memory: 100
max_turns: 50
max_actions: 20
```

Ollama 不需要 API key。若使用其他模型，把 `model` 改成 `api/tags` 中显示的名称。

## 火山 Ark 示例

`.env`：

```env
ARK_API_KEY=your-ark-key
```

`config.local.yaml`：

```yaml
provider: "ark"
model: "doubao-seed-1-8-251228"
api_key: ${ARK_API_KEY}
base_url: "https://ark.cn-beijing.volces.com/api/v3"
```

## 终端运行面板

每轮对话完成后，REPL 会输出类似面板：

```text
+ Runtime --------------------------------------------------+
| Model   qwen/qwen3.5-flash                               |
| Tokens  prompt=1234 completion=128 total=1362 turn=1362  |
| Time    model=1.8s total=2.1s                             |
| Flow    Input -> Memory -> Model -> Tools -> Model -> Out |
| Tools   get_system_model_info ok                          |
| Memory  recent_turns=3 profile=true                       |
| Sandbox enabled=true                                      |
+-----------------------------------------------------------+
```

说明：

- `Tokens` 来自模型返回的 `ResponseMeta.Usage`，不额外向模型提问。
- 如果 provider 没有返回 usage，会显示 `unknown`。
- `Flow` 展示本轮经过的模块；没有工具调用时不会出现 Tools 回路。
- `Sandbox` 表示沙盒目录是否已启用。

## 内置工具

| 工具 | 用途 |
| --- | --- |
| `get_current_time` | 返回本机当前时间 |
| `calculator` | 安全解析基础四则运算 |
| `get_system_model_info` | 查看 provider/model |
| `read_user_profile` / `save_user_profile` | 读取和更新长期用户画像 |
| `schedule_task` / `list_scheduled_tasks` | 创建和查看提醒任务 |
| `modify_scheduled_task` / `delete_scheduled_task` | 修改和删除提醒任务 |
| `list_office_files` | 列出 office 沙盒文件 |
| `read_office_file` | 读取 office 内文件 |
| `write_office_file` | 写入 office 内文件 |
| `execute_office_shell` | 在 office 目录内执行非交互式命令 |

## 沙盒边界

沙盒根目录：

```text
workspace/office
```

允许：

```text
read_office_file("notes/todo.txt")
write_office_file("scripts/demo.py", "...", "w")
execute_office_shell("dir")
```

拒绝：

```text
../outside.txt
C:\Windows\...
cd ..
python -c "..."
node -e "..."
```

模型提示词会要求拒绝越权操作；工具层也会再次校验，避免只依赖模型自觉。

## 任务提醒

任务存储在：

```text
workspace/tasks.json
```

运行 REPL 时，后台心跳会定时检查任务，到期后输出：

```text
[Reminder] drink water
```

支持 `hourly`、`daily`、`weekly` 重复任务和可选 `repeat_count`。

## 动态技能

CyberClaw-Go 会扫描：

```text
workspace/office/skills/*/SKILL.md
workspace/office/skills/*/README.md
```

每个技能会注册为工具，采用 `mode=help` / `mode=run` 两段式使用。`run` 模式中的 `{baseDir}` 会映射到对应技能目录，仍然受沙盒约束。

## 审计日志

日志位置：

```text
workspace/logs/audit_YYYY-MM-DD.json
```

常见事件：

```text
session_turn
tool_call
tool_result
final_response
token_usage
module_flow
```

可以用 PowerShell 查看：

```powershell
Get-Content .\workspace\logs\audit_2026-04-17.json -Tail 20
```

## 测试

运行全部测试：

```powershell
go test ./...
```

只跑沙盒测试：

```powershell
go test ./internal/tools -run Sandbox -v
```

只跑 Agent 运行统计测试：

```powershell
go test ./internal/agent -run Runtime -v
```

## 已知注意事项

- 本地 Ollama 的 `/api/generate` 已验证可用；CyberClaw 使用 Eino 的 Ollama ChatModel，会走 chat/tool-calling 路径，不同本地模型对工具调用的支持可能有差异。
- token 统计依赖 provider 返回 usage；没有 usage 时不做本地估算。
- 当前模块可视化是 REPL 面板，不包含独立 Web dashboard。
- Windows 终端如果通过管道输入中文，可能受编码影响；交互式输入通常更稳定。

## 安全建议

- 不要提交 `.env`、`config.local.yaml`、`workspace/`。
- 不要把真实 API key 写进 README、测试或 `config.yaml`。
- 如果要让模型执行 shell，优先把文件放进 `workspace/office`，并使用非交互式命令。

