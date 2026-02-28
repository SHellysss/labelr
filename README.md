# labelr

Automatically organize your Gmail inbox using AI. labelr runs quietly in the background, reading new emails and applying the right label so you don't have to.

## Why labelr?

Gmail filters are rigid. They match keywords and senders, but can't understand what an email is actually about. labelr uses AI to read your emails and classify them intelligently, the same way you would.

- An invoice from a new vendor? **Finance**.
- A meeting reschedule from your coworker? **Scheduling**.
- A newsletter you subscribed to last week? **Newsletter**.

No rules to write. No filters to maintain. Just labels that make sense.

## Features

- **Set it and forget it** - runs in the background, starts on boot, labels emails as they arrive
- **Works with your AI** - OpenAI, DeepSeek, Groq, Ollama (local), or any OpenAI-compatible endpoint
- **Your labels, your rules** - define custom categories with descriptions to guide the AI
- **Catches up automatically** - handles sleep, reboots, and network outages gracefully
- **Backlog support** - label your past emails in one command
- **Private by design** - everything stays on your machine, no cloud service to trust
- **Cross-platform** - macOS, Linux, and Windows

## Install

```bash
curl -sSL https://raw.githubusercontent.com/Pankaj3112/labelr/main/install.sh | sh
```

Or download a binary from the [releases page](https://github.com/Pankaj3112/labelr/releases).

## Getting Started

### 1. Run the setup wizard

```bash
labelr setup
```

The wizard walks you through everything:

- Connect your Gmail account
- Pick an AI provider and model
- Enter your API key (or skip for Ollama)
- Choose which labels to use
- Optionally test on your 10 most recent emails

The background service starts automatically when setup is complete. You're done.

### 2. Catch up on old emails (optional)

```bash
labelr sync --last 7d
```

Label your recent email history. Use any combination like `30m`, `12h`, `7d`.

## Commands

```
labelr setup                 # First-time setup wizard
labelr start                 # Start the background service
labelr stop                  # Stop the service
labelr status                # See if it's running and queue stats
labelr logs                  # Watch live log output
labelr sync --last <period>  # Label past emails (e.g. 30m, 12h, 7d)
labelr uninstall             # Remove everything
```

The setup wizard starts the service for you. You only need `labelr start` if you've previously stopped it with `labelr stop`.

## Choosing an AI Provider

| Provider | Cost | Privacy | Speed | Setup |
|----------|------|---------|-------|-------|
| **Ollama** | Free | Fully local | Depends on hardware | [Install Ollama](https://ollama.com) first |
| **Groq** | Free tier available | Cloud | Very fast | Get an API key at [groq.com](https://groq.com) |
| **DeepSeek** | Very cheap | Cloud | Fast | Get an API key at [deepseek.com](https://deepseek.com) |
| **OpenAI** | Pay per use | Cloud | Fast | Get an API key at [platform.openai.com](https://platform.openai.com) |
| **Custom** | Varies | Varies | Varies | Any OpenAI-compatible API - provide your own base URL |

Token usage is minimal. Each email costs roughly 100-300 input tokens and ~10 output tokens. Even with heavy email volume, costs stay well under $1/month with cloud providers.

## Default Labels

These work well out of the box:

| Label | What it catches |
|-------|----------------|
| **Action Required** | Emails that need a reply or action from you |
| **Informational** | FYI emails, no action needed |
| **Newsletter** | Newsletters and subscriptions |
| **Finance** | Bills, receipts, bank notifications |
| **Scheduling** | Calendar invites, meeting requests |
| **Personal** | Messages from friends and family |
| **Automated** | Notifications, alerts, system emails |

You can customize these during setup or add your own. Write clear descriptions for each label so the AI knows how to classify.

## How It Works

1. **Poll** - every 60 seconds, labelr checks Gmail for new messages
2. **Classify** - sends the sender, subject, and a short body preview to your AI provider
3. **Label** - the AI picks the best label and labelr applies it in Gmail

Everything is queued locally in a SQLite database, so nothing gets lost if your computer goes to sleep or loses internet.

## Privacy

All data stays on your machine in `~/.labelr/`. The only external calls are to Gmail (to read and label emails) and your chosen AI provider (to classify them). If you use Ollama, everything stays completely local.

## Platform Support

| OS | Service Manager | Auto-start on boot |
|----|----------------|--------------------|
| macOS | launchd | Yes |
| Linux | systemd | Yes |
| Windows | Task Scheduler | Yes |

## Building from Source

```bash
git clone https://github.com/Pankaj3112/labelr.git
cd labelr
make build
./bin/labelr setup
```

Requires Go 1.25+.

## License

MIT
