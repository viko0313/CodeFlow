# CodeFlow Agent

CodeFlow Agent is a terminal-native enterprise coding assistant built with Go and CloudWeGo Eino. The product direction is to match the core workflow of Claude Code: start in a project directory, keep the CLI as the primary interface, make privileged actions visible and confirmable, and use a web UI only as a later visual companion.

This repository now uses the CodeFlow V2 architecture and naming throughout the codebase. Phase 1 introduces the `codeflow` CLI, project-root sessions, real Redis/PostgreSQL storage boundaries, permission-gated tools, and a clearer package layout.

## Current Phase

Phase 1 MVP is intentionally focused:

- `codeflow` Cobra CLI with `start`, `session`, `config`, and `version` commands.
- Project-root sessions created from the directory where `codeflow start` runs.
- `AGENT.md` project rules loaded into the assistant context.
- Redis short-term memory for recent turns.
- PostgreSQL session metadata storage.
- Permission-gated project-root file writes and shell execution.
- Diff preview before file writes.
- Shell command preview, working directory, timeout, and risk display before execution.
- JSONL audit events under `.codeflow/logs`.
- Single primary CLI entrypoint under `cmd/codeflow`.
- Local Web workspace with Dashboard, IDE, WebSocket approvals, Monaco editor, and Xterm.js terminal.

Still deferred: Milvus vector memory, full MCP integration, Subagents, checkpoint rewind, and the self-evolution engine. The V2 packages reserve clean boundaries for those phases without pretending they are complete.

## Quick Start

Start infrastructure:

```powershell
docker compose up -d postgres redis
```

Set the storage DSN:

```powershell
$env:CODEFLOW_POSTGRES_DSN="postgres://codeflow:codeflow@localhost:5432/codeflow?sslmode=disable"
$env:CODEFLOW_REDIS_ADDR="localhost:6379"
```

Set an LLM key, for example Ark:

```powershell
$env:ARK_API_KEY="your-key"
```

Run the new CLI:

```powershell
go run .\cmd\codeflow version
go run .\cmd\codeflow start
```

Build a binary:

```powershell
go build -o codeflow.exe .\cmd\codeflow
.\codeflow.exe start
```

## CLI Commands

```text
codeflow start
codeflow session list
codeflow session switch <session-id>
codeflow session delete <session-id>
codeflow config get <key>
codeflow config set <key> <value>
codeflow version
```

Inside `codeflow start`:

```text
/help
/clear
/version
/session list
/session switch <session-id>
/run <command>
/edit <path>
/diff
/exit
```

## Web UI

CodeFlow also ships a local Web workspace built with Next.js, Hertz, WebSocket,
Monaco, Xterm.js, Auth.js, TanStack Query, Zustand, Shadcn-style components, and
Recharts.

Start the Go Web API and WebSocket server:

```powershell
go run .\cmd\codeflow web --addr localhost:8742
```

Start the frontend:

```powershell
cd web
npm install
npm run dev
```

Open:

```text
http://localhost:3000
```

Local development login defaults to:

```text
CODEFLOW_WEB_USER=admin
CODEFLOW_WEB_PASSWORD=codeflow
```

Set `AUTH_SECRET` for non-development runs. The frontend proxies
`/api/codeflow/*` to `http://localhost:8742/api/*`; WebSocket traffic connects
directly to `ws://localhost:8742/api/ws`.

The Web IDE defaults to Eino ReAct mode. Use the `Plan` toggle in the IDE to ask
the agent to produce a concise plan before execution-oriented guidance. Document
upload is available from the IDE toolbar; files are stored under
`.codeflow/uploads` and parsed through EinoExt's file document loader before the
content is preloaded into the next chat request.

Skill and MCP preloading are configured in `.codeflow/config.yaml`:

```yaml
agent:
  mode: "react"
  plan_enabled: false
skills:
  enabled: true
  dirs:
    - ".codeflow/skills"
  preload: []
  max_content_bytes: 6000
mcp:
  enabled: true
  preload: true
  config_files:
    - ".codeflow/mcp.json"
    - ".codeflow/mcp.yaml"
  servers: []
documents:
  upload_dir: ".codeflow/uploads"
  max_upload_bytes: 10485760
  allowed_extensions: [".txt", ".md", ".markdown", ".json", ".yaml", ".yml", ".csv", ".html", ".pdf"]
```

`skills.preload` accepts skill names or `*`. MCP config files support the common
`mcpServers` JSON shape, for example:

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "node",
      "args": ["server.js"],
      "env": { "ROOT": "." }
    }
  }
}
```

## Configuration

Project configuration lives in:

```text
.codeflow/config.yaml
```

CodeFlow creates a safe default file on first start. Secrets must be environment references, not plaintext values:

```yaml
provider: "ark"
model: "doubao-seed-1-8-251228"
api_key: ${ARK_API_KEY}
base_url: "https://ark.cn-beijing.volces.com/api/v3"
storage:
  postgres_dsn: ${CODEFLOW_POSTGRES_DSN}
  redis_addr: "localhost:6379"
  redis_password: ${CODEFLOW_REDIS_PASSWORD}
  redis_db: 0
permissions:
  trusted_commands: []
  trusted_dirs: []
  writable_dirs: []
runtime:
  max_turns: 50
  max_actions: 20
```

`AGENT.md` in the project root stores project-specific rules such as coding style, common commands, architecture notes, and deployment constraints. CodeFlow reads it when starting a session.

## V2 Package Layout

```text
cmd/codeflow/                    CodeFlow CLI binary
internal/codeflow/cli/           Cobra commands and REPL
internal/codeflow/config/        Project/global config loading and secret policy
internal/codeflow/engine/        Event-stream engine boundary
internal/codeflow/session/       Session interfaces
internal/codeflow/storage/       PostgreSQL session store
internal/codeflow/memory/        Redis short-term memory
internal/codeflow/permission/    Permission review and safety validation
internal/codeflow/tools/         Project-root tool executor
internal/codeflow/audit/         JSONL audit event writer
internal/agent/                  Existing Eino ReAct core retained for reuse
internal/model/                  Existing multi-provider model factory
```

## Safety Model

Phase 1 treats the project root as the work area, but privileged actions are never silent:

- File paths must stay inside the project root.
- File writes show a diff before writing.
- Shell commands show command, working directory, timeout, and risk before execution.
- Dangerous shell patterns such as `python -c`, `node -e`, parent traversal, and recursive destructive deletion are blocked.
- Allow lists can skip confirmation for trusted commands or directories, but audit logging still records the operation.
- API keys, tokens, Redis passwords, and PostgreSQL DSNs must come from environment variables.

## Development

Run all tests:

```powershell
go test ./...
```

Run integration tests against local services:

```powershell
$env:CODEFLOW_TEST_POSTGRES_DSN="postgres://codeflow:codeflow@localhost:5432/codeflow?sslmode=disable"
$env:CODEFLOW_TEST_REDIS_ADDR="localhost:6379"
go test ./internal/codeflow/...
```

## Roadmap

- Phase 1: CLI, sessions, permissions, Redis/PostgreSQL, audit, V2 skeleton.
- Phase 2: progressive Skill disclosure, MCP configuration, and layered memory expansion.
- Phase 3: checkpoint rewind, Git/code-understanding tools, and Subagent orchestration.
- Phase 4: self-evolution engine, feedback learning, code pattern learning, and performance tuning.
- Phase 5: Harden Web GUI synchronization, multi-client collaboration, and deployment packaging.
- Phase 6: hardening, cross-platform packaging, security testing, and official Skill library.
