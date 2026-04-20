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

Not in Phase 1: Web GUI, Milvus vector memory, full MCP integration, Subagents, checkpoint rewind, and the self-evolution engine. The V2 packages reserve clean boundaries for those phases without pretending they are complete.

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
- Phase 5: Next.js Web GUI, Monaco, Xterm.js, and real-time CLI/Web synchronization.
- Phase 6: hardening, cross-platform packaging, security testing, and official Skill library.
