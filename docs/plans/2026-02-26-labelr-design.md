# labelr - Design Document

**Date**: 2026-02-26
**Status**: Approved

## What We're Building

A local-first CLI tool that polls Gmail for new emails, classifies them using AI, and applies labels automatically. Single binary, runs as a background daemon, zero infrastructure.

## Stack

| Component | Choice |
|-----------|--------|
| Language | Go |
| CLI | cobra + bubbletea |
| AI | OpenAI Go SDK (works with OpenAI, Ollama, DeepSeek, Groq via compatible endpoints) |
| Gmail | google.golang.org/api/gmail/v1 |
| Database | SQLite via modernc.org/sqlite (pure Go) |
| Model catalog | Models.dev API (JSON endpoint) |
| Config | ~/.labelr/ directory |

## Key Design Decisions

- **OpenAI SDK over Jetify AI SDK**: Battle-tested, and OpenAI-compatible endpoints cover Ollama, DeepSeek, Groq. Can swap to Jetify later.
- **No confidence threshold**: LLMs are reliable enough. Always apply the AI's best pick.
- **Single label per email**: No multi-label complexity.
- **Structured output**: Constrain AI response via JSON schema/enum to prevent hallucinated labels.
- **Plain token storage**: File with chmod 600, same as gh/gcloud/aws CLIs.
- **API key**: Stored in config, env var overrides if set.
- **Plain label names**: No prefix. Reuse existing Gmail labels if they match.
- **Monolithic binary**: Single `labelr` binary with hidden `daemon` subcommand.
- **Personal tool first**: Optimize for the author's use case, open-source for others.

## Architecture

### Two Decoupled Loops

```
labelr binary
├── CLI commands (init, start, stop, status, logs, config, sync, uninstall)
└── Hidden "daemon" subcommand
    ├── Poller goroutine (every 60s)
    │   └── Gmail history.list → insert message IDs into SQLite
    └── Worker goroutine (continuous)
        └── SQLite pending → fetch email → AI classify → apply Gmail label
```

**Poller** (every 60 seconds):
1. Read last `historyId` from SQLite `state` table
2. Call `users.history.list` filtered by INBOX + messageAdded
3. Insert new message IDs into `messages` table with status `pending`
4. Update `historyId`

**Worker** (continuous):
1. Query `messages` where status = `pending`, ordered by `created_at`
2. Set status to `processing`
3. Fetch email via `users.messages.get`
4. Extract subject, from, first ~500 chars of plain text body
5. Call AI with structured output → get label name
6. Apply label via `users.messages.modify`
7. Set status to `labeled`
8. On error: increment attempts, if >= 3 set `failed`, else back to `pending`
9. If no pending messages, sleep 1-2s and re-check

Communication between loops: none. Decoupled via SQLite.

### Startup Behavior
- Reset any `processing` messages to `pending`
- Validate config
- Verify/refresh Gmail token
- Start poller + worker as goroutines
- Block on SIGTERM/SIGINT for graceful shutdown

## Database Schema

### Table: messages
```sql
CREATE TABLE messages (
    id          TEXT PRIMARY KEY,
    thread_id   TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',
    label       TEXT,
    attempts    INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    processed_at TEXT
);
CREATE INDEX idx_messages_status ON messages(status);
```

### Table: state
```sql
CREATE TABLE state (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
```
Stores: `history_id`, `last_poll_time`

### Table: label_mappings
```sql
CREATE TABLE label_mappings (
    name     TEXT PRIMARY KEY,
    gmail_id TEXT NOT NULL
);
```

## AI Classification

### Providers (all OpenAI-compatible)
- OpenAI (native)
- Ollama (local, free, http://localhost:11434/v1)
- DeepSeek
- Groq
- Custom (user provides base URL)

### Prompt
```
You are an email classifier. Given the email details below, classify it into exactly one of the provided labels.

Email:
- From: {sender}
- Subject: {subject}
- Body preview: {first 500 chars}

Available labels:
{JSON array of label objects with name + description}

Respond with a JSON object: {"label": "<label_name>"}
```

Uses structured output (JSON mode + response schema) to constrain label to configured enum values.

### Token Efficiency
- ~100-300 input tokens, ~10 output tokens per email
- At gpt-4o-mini pricing: ~$0.00002 per email

## Gmail Integration

### OAuth Flow
1. Embed client ID + secret in binary
2. Start temp HTTP server on localhost (random port)
3. Open browser → Google consent → callback
4. Exchange code for access + refresh tokens
5. Store in ~/.labelr/credentials.json (chmod 600)

Scopes: `gmail.modify`, `gmail.labels`

### Polling
- `users.history.list` with startHistoryId, labelId=INBOX, historyTypes=messageAdded
- On 404 (expired): fall back to `users.messages.list` for recent emails

### Labels
- Create via `users.labels.create`, handle 409 (already exists)
- Store name → Gmail ID in `label_mappings` table
- Apply via `users.messages.modify` with addLabelIds

## CLI Commands

```
labelr init        Interactive setup wizard
labelr start       Install + start background service
labelr stop        Stop background service
labelr status      Show daemon state, queue stats
labelr logs        Tail daemon log file
labelr config      Interactive config editor
labelr sync        One-time backlog (--last 7d)
labelr uninstall   Remove service + cleanup
labelr daemon      [hidden] Run daemon in foreground
```

### labelr init flow
1. Welcome message
2. Gmail OAuth (browser)
3. Provider selection (bubbletea)
4. Model selection (Models.dev → bubbletea)
5. API key (auto-detect env or prompt)
6. Label config (show defaults, customize)
7. Create labels in Gmail
8. Ask: "Label 10 recent emails to test?"
9. Write config

### Service Management
- **macOS**: launchd plist in ~/Library/LaunchAgents/
- **Linux**: systemd user unit in ~/.config/systemd/user/
- **Windows**: Task Scheduler via schtasks

## Project Structure

```
labelr/
├── cmd/labelr/main.go
├── internal/
│   ├── cli/
│   │   ├── init.go, start.go, stop.go, status.go
│   │   ├── logs.go, config.go, sync.go, uninstall.go
│   │   └── daemon.go
│   ├── daemon/
│   │   ├── daemon.go, poller.go, worker.go
│   ├── gmail/
│   │   ├── auth.go, client.go
│   ├── ai/
│   │   ├── classifier.go, providers.go
│   ├── db/
│   │   ├── db.go, migrations.go
│   ├── config/
│   │   └── config.go
│   ├── service/
│   │   ├── service.go, launchd.go, systemd.go, taskscheduler.go
│   └── log/
│       └── log.go
├── go.mod, go.sum
└── Makefile
```

## Config (~/. labelr/config.json)

```json
{
  "gmail": { "email": "user@gmail.com" },
  "ai": {
    "provider": "openai",
    "model": "gpt-4o-mini",
    "apiKey": "sk-...",
    "baseURL": "https://api.openai.com/v1"
  },
  "labels": [
    {"name": "Action Required", "description": "Emails requiring a response or action"},
    {"name": "Informational", "description": "FYI emails, no action needed"},
    {"name": "Newsletter", "description": "Newsletters, digests, mailing lists"},
    {"name": "Finance", "description": "Bills, receipts, banking, payments"},
    {"name": "Scheduling", "description": "Calendar invites, meeting requests"},
    {"name": "Personal", "description": "Personal emails from friends and family"},
    {"name": "Automated", "description": "Automated notifications, alerts, system emails"}
  ],
  "pollInterval": 60
}
```

## Error Handling

| Scenario | Handling |
|----------|----------|
| Computer off/reboot | Service auto-restarts. history.list catches up. |
| Crash mid-processing | Reset processing→pending on restart. SQLite durable. |
| No internet | Fail gracefully, retry next cycle. |
| AI provider down | Increment attempts, fail after 3. |
| History expired (30+ days) | Fall back to messages.list, store fresh historyId. |
| Token revoked | Log error, stop daemon. User re-auths via init. |
| Label exists in Gmail | Reuse it, store its ID. |
| Duplicate message ID | SQLite PK prevents duplicates. |
| Config corrupted | Refuse to start, clear error message. |
| Log file too large | Size-based rotation: 10MB max, 3 rotations. |

## Out of Scope (v1)

- Draft response generation
- Multiple Gmail accounts
- Web UI / desktop app
- Analytics / dashboards
- Outlook / IMAP
- Mobile app
