# CodeFlow Phase 1 Architecture

This document describes the implemented Phase 1 boundary for the CodeFlow V2 migration.

## Implemented

- `cmd/codeflow` is the primary CLI binary.
- `cmd/codeflow` is the only CLI entrypoint.
- `internal/codeflow/cli` owns Cobra commands and the REPL.
- `internal/codeflow/config` owns `.codeflow/config.yaml`, environment expansion, and plaintext secret rejection.
- `internal/codeflow/storage` stores session metadata in PostgreSQL.
- `internal/codeflow/memory` stores short-term turns in Redis.
- `internal/codeflow/permission` performs path validation, shell validation, allow-list checks, and confirmation.
- `internal/codeflow/tools` executes project-root file writes and shell commands only after permission review.
- `internal/codeflow/engine` exposes an event-stream runner boundary over the existing model factory.

## Deferred

- Progressive Skill disclosure and MCP activation are Phase 2.
- Checkpoint rewind, Git/code-understanding tools, and Subagents are Phase 3.
- Self-evolution and performance tuning are Phase 4.
- Web GUI and CLI/Web synchronization are Phase 5.

## Runtime Contract

`codeflow start` uses the current directory as the project root. It requires PostgreSQL and Redis, reads `AGENT.md` when present, creates or resumes an active project session, and writes audit events below `.codeflow/logs`.

Privileged operations are never executed directly from model text. In Phase 1, privileged local operations are exposed through REPL slash commands such as `/run` and `/edit`, both of which route through the permission gate.
