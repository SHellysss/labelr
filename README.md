# labelr

A local-first CLI tool that automatically classifies and labels your Gmail emails using AI. Runs as a lightweight background daemon on your machine — no cloud infrastructure required.

## How It Works

labelr polls your Gmail inbox for new emails, sends them to an AI model for classification, and applies the appropriate label — all running locally on your computer.

```
┌──────────┐      ┌──────────┐      ┌──────────┐      ┌──────────┐
│  Gmail    │─────▶│  Poller  │─────▶│  Worker  │─────▶│    AI    │
│  Inbox    │      │ (60s)    │      │          │      │ Classify │
└──────────┘      └────┬─────┘      └────┬─────┘      └────┬─────┘
                       │                 │                  │
                       ▼                 ▼                  ▼
                  ┌──────────┐     ┌──────────┐      ┌──────────┐
                  │  SQLite  │     │  Fetch &  │      │  Apply   │
                  │  Queue   │     │  Extract  │      │  Label   │
                  └──────────┘     └──────────┘      └──────────┘
```

**Poller** checks Gmail every 60 seconds for new messages and queues them in a local SQLite database. **Worker** picks up pending messages, extracts the sender, subject, and body preview, sends them to your chosen AI provider, and applies the returned label to the email in Gmail.

## Features

- **Multiple AI providers** — OpenAI, DeepSeek, Groq, or Ollama (fully local)
- **Structured output** — JSON schema-constrained classification for reliable labeling
- **Background daemon** — runs as a system service (launchd, systemd, or Task Scheduler)
- **Automatic recovery** — catches up after sleep, reboots, or network outages via Gmail history API
- **Backlog sync** — classify your last 1, 7, or 30 days of email in one command
- **Customizable labels** — define your own categories with descriptions to guide the AI
- **Interactive setup** — guided wizard handles OAuth, AI config, and label creation
- **Cross-platform** — macOS, Linux, and Windows

## Quick Start

### Install

**From source:**

```bash
git clone https://github.com/Pankaj3112/labelr.git
cd labelr
make build
```

**From releases:**

```bash
curl -sSL https://github.com/Pankaj3112/labelr/releases/latest/download/install.sh | sh
```

### Setup

```bash
labelr init
```

The interactive wizard walks you through:

1. **Gmail authentication** — opens your browser for OAuth consent
2. **AI provider** — choose OpenAI, DeepSeek, Groq, or Ollama
3. **Model selection** — pick from available models with structured output support
4. **API key** — enter your key (or skip for Ollama)
5. **Labels** — accept defaults or define custom categories
6. **Test run** — optionally classify 10 recent emails to verify

### Run

```bash
# Start the background daemon
labelr start

# Check status
labelr status

# View live logs
labelr logs

# Sync past emails
labelr sync --last 7d

# Stop the daemon
labelr stop
```

## Commands

| Command | Description |
|---------|-------------|
| `labelr init` | Interactive first-time setup |
| `labelr start` | Install and start the background service |
| `labelr stop` | Stop the background service |
| `labelr status` | Show daemon state and queue statistics |
| `labelr logs` | Tail the daemon log file |
| `labelr config` | Interactively edit configuration |
| `labelr sync --last <duration>` | One-time backlog scan (`1d`, `7d`, `30d`) |
| `labelr uninstall` | Remove the service and clean up all data |

## Configuration

All configuration is stored in `~/.labelr/config.json`, created during `labelr init`. You can edit it interactively with `labelr config`.

```json
{
  "gmail": {
    "email": "you@gmail.com"
  },
  "ai": {
    "provider": "openai",
    "model": "gpt-4o-mini",
    "apiKey": "sk-...",
    "baseURL": "https://api.openai.com/v1"
  },
  "labels": [
    { "name": "Action Required", "description": "Emails requiring a response or action" },
    { "name": "Informational", "description": "FYI emails, no action needed" },
    { "name": "Newsletter", "description": "Newsletters and subscriptions" },
    { "name": "Finance", "description": "Bills, receipts, and financial updates" },
    { "name": "Scheduling", "description": "Calendar invites and meeting requests" },
    { "name": "Personal", "description": "Personal emails from friends and family" },
    { "name": "Automated", "description": "Notifications, alerts, and automated messages" }
  ],
  "pollInterval": 60
}
```

### Custom Labels

Define as many labels as you need. Each label has a name (applied in Gmail) and a description (used as context for the AI classifier). Be specific in descriptions — the AI uses them to decide which label fits best.

## AI Providers

| Provider | Base URL | Models | Local |
|----------|----------|--------|-------|
| OpenAI | `https://api.openai.com/v1` | gpt-4o-mini, gpt-4o, etc. | No |
| DeepSeek | `https://api.deepseek.com/v1` | deepseek-chat, etc. | No |
| Groq | `https://api.groq.com/openai/v1` | mixtral, llama, etc. | No |
| Ollama | `http://localhost:11434/v1` | Any local model | Yes |

All providers use an OpenAI-compatible API. Token usage is minimal — roughly 100-300 input tokens and ~10 output tokens per email.

## Environment Variables

| Variable | Description |
|----------|-------------|
| `OPENAI_API_KEY` | OpenAI API key |
| `DEEPSEEK_API_KEY` | DeepSeek API key |
| `GROQ_API_KEY` | Groq API key |
| `GOOGLE_CLIENT_ID` | Gmail OAuth client ID (build-time) |
| `GOOGLE_CLIENT_SECRET` | Gmail OAuth client secret (build-time) |

API keys can also be stored in the config file. Environment variables take precedence.

## Data Storage

Everything lives in `~/.labelr/`:

| File | Purpose |
|------|---------|
| `config.json` | Configuration (labels, AI provider, poll interval) |
| `credentials.json` | OAuth tokens (chmod 600) |
| `labelr.db` | SQLite database (message queue, state, label mappings) |
| `logs/daemon.log` | Daemon logs (10MB rotation, 3 backups) |
| `models-cache.json` | Cached model list from models.dev (7-day TTL) |

## Service Management

labelr registers as a system service appropriate for your OS:

- **macOS** — launchd plist in `~/Library/LaunchAgents/`
- **Linux** — systemd user unit in `~/.config/systemd/user/`
- **Windows** — Task Scheduler entry

The daemon auto-restarts on reboot and recovers gracefully from crashes by resetting in-progress messages back to pending.

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Computer off / reboot | Service auto-restarts, history API catches up |
| Crash mid-processing | Resets processing → pending on restart |
| No internet | Fails gracefully, retries next poll cycle |
| AI provider down | Retries up to 3 times, then marks as failed |
| Gmail history expired (30+ days) | Falls back to `messages.list` |
| OAuth token revoked | Logs error, stops daemon (re-auth via `labelr init`) |
| Duplicate message | SQLite primary key prevents duplicates |
| Log file too large | Automatic rotation at 10MB |

## Development

```bash
# Build
make build

# Run (with Google credentials via ldflags)
make run

# Run tests
make test

# Clean build artifacts
make clean

# Cross-platform release build
make release
```

### Project Structure

```
labelr/
├── cmd/labelr/          # Entry point
├── internal/
│   ├── cli/             # Command implementations (init, start, stop, etc.)
│   ├── daemon/          # Background service (poller + worker loops)
│   ├── gmail/           # Gmail API client and OAuth flow
│   ├── ai/              # AI classifier and provider configs
│   ├── db/              # SQLite database layer
│   ├── config/          # Config loading and saving
│   ├── service/         # OS service management (launchd, systemd, taskscheduler)
│   ├── log/             # File-based logger with rotation
│   └── ui/              # Terminal styling utilities
├── docs/plans/          # Design and implementation documents
├── Makefile             # Build targets
├── .goreleaser.yml      # Release configuration
└── install.sh           # Installation script
```

## License

MIT
