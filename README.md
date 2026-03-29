# 🧭 Bitácora

**Ship's log for AI agents. Persistent memory that survives between sessions.**

Your AI coding agent forgets everything when the session ends. Every decision, every bug fix, every pattern — gone. The next session starts from zero.

Bitácora fixes that. One binary. One SQLite file. Zero dependencies.

```
Agent (Claude Code / Cursor / VS Code / Windsurf / ...)
    ↓ MCP stdio
Bitácora (single Go binary)
    ↓
SQLite + FTS5 (~/.bitacora/memory.db)
```

> **bitácora** /bi.ˈta.ko.ɾa/ — *nautical*: the ship's logbook where the captain records course decisions, problems encountered, and lessons learned during the voyage.

---

## Quick start

### Install via Homebrew (macOS)

```bash
brew tap florelmx/tap
brew install bitacora
```

### Install via Go

```bash
go install github.com/florelmx/bitacora/cmd/bitacora@latest
```

### Download binary

Grab the latest release for your platform from [GitHub Releases](https://github.com/florelmx/bitacora/releases).

### Configure Claude Code

```bash
bitacora setup
```

That's it. This registers the MCP server, hooks, and memory instructions automatically.

For project-level config (committable to your repo):

```bash
bitacora setup --project
```

---

## How it works

### The agent saves, Bitácora stores

```
1. Agent completes significant work (bugfix, architecture decision, etc.)
2. Agent calls bit_save with a structured summary
3. Bitácora persists to SQLite with FTS5 indexing
4. Next session: agent gets full context automatically
```

### Session lifecycle

```
Session starts → Hook injects previous context automatically
                     ↓
Agent works → Saves decisions, bugs, patterns proactively
                     ↓
Session ends → Hook generates summary + snapshot
                     ↓
Next session → Full context restored, as if it never left
```

### Two automation layers

**Layer 1 — Hooks (guaranteed):** Claude Code hooks execute automatically at session start, end, and before compaction. The agent doesn't need to "remember" to do this — it happens every time.

**Layer 2 — CLAUDE.md (agent-driven):** Instructions guide the agent to search memory before decisions and save observations after significant work. This layer enhances but doesn't replace the hooks.

---

## Features

### Three scope levels

| Scope | What it stores | Example |
|-------|---------------|---------|
| **global** | Conventions that apply everywhere | "Always use conventional commits" |
| **project** | Decisions specific to one repo | "Use JWT for auth in this API" |
| **workspace** | Shared across a monorepo | "All microservices use gRPC" |

Context loading combines all three: project (most specific) → workspace → global.

### Memory relationships

Observations connect to each other through typed relations:

- `caused_by` — "This decision was made because of this bug"
- `supersedes` — "This replaces the old approach" (marks old as inactive)
- `relates_to` — "These are connected"
- `contradicts` — "This conflicts with that" (triggers warning)
- `depends_on` — "This needs that to work"
- `derived_from` — "This evolved from that"

### Relevance decay + reinforcement

Every observation has a `relevance_score` that decays 1% daily without access but increases +0.15 each time it's consulted. Frequently used memories stay fresh; old unused ones fade naturally without being deleted.

### Full-text search (FTS5)

Search across all observations in milliseconds using SQLite's FTS5 engine. Supports:
- Multiple words: `"bug authentication"`
- Unicode with accents: `"autenticacion"` finds `"autenticación"`
- Category filters: search only bugs, only decisions, etc.
- BM25 ranking combined with relevance score

### Compaction safety net

When Claude Code compacts (compresses) a long conversation, Bitácora captures a snapshot before information is lost. The `PreCompact` hook saves the session state automatically.

---

## MCP tools (bit_*)

| Tool | Purpose |
|------|---------|
| `bit_start_session` | Start session, auto-load project context |
| `bit_end_session` | End session with summary |
| `bit_save` | Save observation (decision, bug, pattern, note, preference) |
| `bit_search` | Full-text search with FTS5 |
| `bit_context` | Get complete context in one call |
| `bit_get` | Get full observation + relations |
| `bit_save_request` | Save user request with priority tracking |
| `bit_update_request` | Update request status (pending → completed) |
| `bit_relate` | Create relation between observations |
| `bit_stats` | Memory system statistics |
| `bit_list_sessions` | List sessions by project/status |

### Progressive disclosure

Token-efficient retrieval in 3 layers:

```
1. bit_search "auth middleware"  → compact results with IDs (~100 tokens each)
2. bit_browse with filters       → chronological context around a result
3. bit_get id=42                 → full content + relations
```

---

## CLI

```bash
bitacora mcp              # Start MCP server (stdio)
bitacora context           # Print recent context (used by SessionStart hook)
bitacora search "query"    # Search memory from terminal
bitacora stats             # Show memory statistics
bitacora end-session       # End active session (used by SessionEnd hook)
bitacora setup             # Configure Claude Code globally
bitacora setup --project   # Configure at project level
bitacora version           # Show version
```

### Search flags

```bash
bitacora search "auth" --project mi-app --category bug --limit 5
bitacora search "auth" -p mi-app -c bug -l 5   # short form
```

---

## Agent setup

### Claude Code

```bash
bitacora setup
```

Or manually add to `~/.claude/settings.json`:

```json
{
  "mcpServers": {
    "bitacora": {
      "command": "/path/to/bitacora",
      "args": ["mcp"]
    }
  }
}
```

### Cursor

Add to `.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "bitacora": {
      "command": "bitacora",
      "args": ["mcp"]
    }
  }
}
```

### VS Code / Windsurf / Any MCP agent

Same pattern — point your agent's MCP config to `bitacora mcp` via stdio transport.

---

## Database

All data lives in a single SQLite file: `~/.bitacora/memory.db`

Override the location with:

```bash
export BITACORA_DIR=/custom/path
```

### Schema

| Table | Purpose |
|-------|---------|
| `projects` | Known projects (by git remote) |
| `sessions` | Work sessions with summaries |
| `observations` | Decisions, bugs, patterns, notes, preferences |
| `observations_fts` | FTS5 full-text search index |
| `relations` | Knowledge graph between observations |
| `user_requests` | User requests with priority tracking |
| `requests_fts` | FTS5 index for requests |
| `compaction_snapshots` | Safety net before compaction |

---

## Project structure

```
bitacora/
├── cmd/bitacora/          # Entry point
│   └── main.go
├── internal/
│   ├── db/                # SQLite + FTS5 operations
│   │   ├── db.go
│   │   ├── schema.go
│   │   ├── operations.go
│   │   └── operations_test.go
│   ├── mcp/               # MCP server + tool handlers
│   │   ├── server.go
│   │   └── handlers.go
│   ├── models/            # Data structures
│   │   └── types.go
│   └── setup/             # Claude Code auto-configuration
│       └── setup.go
├── .github/workflows/     # CI/CD
│   └── release.yml
├── .goreleaser.yaml
├── LICENSE
└── README.md
```

---

## Tech stack

| Component | Technology | Why |
|-----------|-----------|-----|
| Language | Go | Single binary, cross-compilation, <15ms startup |
| Database | SQLite (modernc.org/sqlite) | Pure Go, no CGO, portable, serverless |
| Search | FTS5 + BM25 | Built into SQLite, millisecond full-text search |
| MCP SDK | mark3labs/mcp-go | Community SDK for Go, stdio transport |
| CLI | cobra | Standard Go CLI framework with subcommands |
| Distribution | GoReleaser + Homebrew | Automated multi-OS builds |

---

## Inspired by

[Engram](https://github.com/Gentleman-Programming/engram) by Gentleman Programming — the project that pioneered persistent memory for AI coding agents with Go + SQLite + FTS5. Bitácora builds on that foundation with additional features: multi-level scoping, memory relationships, relevance decay, contradiction detection, and compaction safety nets.

---

## Attribution

If you use Bitácora in your project, a mention in your README or documentation is appreciated. Not required by the license, but valued by the community.

---

## License

[MIT](LICENSE)