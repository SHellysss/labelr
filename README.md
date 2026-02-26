# labelr

Automatically organize your Gmail inbox using AI. labelr runs quietly in the background on your computer, reading new emails and applying the right label — so you don't have to.

## Why labelr?

Gmail filters are rigid — they match keywords and senders, but can't understand what an email is actually about. labelr uses AI to read your emails and classify them intelligently, the same way you would.

- An invoice from a new vendor? **Finance**.
- A meeting reschedule from your coworker? **Scheduling**.
- A newsletter you subscribed to last week? **Newsletter**.

No rules to write. No filters to maintain. Just labels that make sense.

## Features

- **Set it and forget it** — runs in the background, starts on boot, labels emails as they arrive
- **Works with your AI** — OpenAI, DeepSeek, Groq, or run fully local with Ollama
- **Your labels, your rules** — define custom categories with descriptions to guide the AI
- **Catches up automatically** — handles sleep, reboots, and network outages gracefully
- **Backlog support** — label your last 1, 7, or 30 days of email in one command
- **Private by design** — everything stays on your machine, no cloud service to trust
- **Cross-platform** — macOS, Linux, and Windows

## Install

```bash
curl -sSL https://raw.githubusercontent.com/Pankaj3112/labelr/main/install.sh | sh
```

Or download a binary directly from the [releases page](https://github.com/Pankaj3112/labelr/releases).

## Getting Started

### 1. Run the setup wizard

```bash
labelr init
```

This walks you through everything interactively:

- **Connect your Gmail** — opens your browser to sign in with Google
- **Pick an AI provider** — OpenAI, DeepSeek, Groq, or Ollama (local)
- **Choose a model** — labelr shows you available models to pick from
- **Enter your API key** — or skip this step if you're using Ollama
- **Set up labels** — use the defaults or create your own categories
- **Test it out** — optionally classify 10 recent emails to see it in action

### 2. Start labeling

```bash
labelr start
```

That's it. labelr installs itself as a background service and starts labeling new emails as they come in. It survives reboots and picks up right where it left off.

### 3. Catch up on old emails (optional)

```bash
labelr sync --last 7d
```

Label your recent email history. Supports `1d`, `7d`, or `30d`.

## Usage

```bash
labelr init                  # First-time setup
labelr start                 # Start the background service
labelr stop                  # Stop the service
labelr status                # Check if it's running and see queue stats
labelr logs                  # Watch the live log output
labelr sync --last <period>  # Label past emails (1d, 7d, 30d)
labelr config                # Change settings after setup
labelr uninstall             # Remove everything
```

## Choosing an AI Provider

| Provider | Cost | Privacy | Speed | Setup |
|----------|------|---------|-------|-------|
| **Ollama** | Free | Fully local | Depends on hardware | [Install Ollama](https://ollama.com) first |
| **Groq** | Free tier available | Cloud | Very fast | Get an API key at [groq.com](https://groq.com) |
| **DeepSeek** | Very cheap | Cloud | Fast | Get an API key at [deepseek.com](https://deepseek.com) |
| **OpenAI** | Pay per use | Cloud | Fast | Get an API key at [platform.openai.com](https://platform.openai.com) |

Token usage is minimal — each email costs roughly 100-300 input tokens and ~10 output tokens. Even with heavy email volume, costs stay well under $1/month with cloud providers.

## Custom Labels

The default labels work well out of the box:

| Label | What it catches |
|-------|----------------|
| **Action Required** | Emails that need a reply or action from you |
| **Informational** | FYI emails, no action needed |
| **Newsletter** | Newsletters and subscriptions |
| **Finance** | Bills, receipts, bank notifications |
| **Scheduling** | Calendar invites, meeting requests |
| **Personal** | Messages from friends and family |
| **Automated** | Notifications, alerts, system emails |

You can fully customize these during `labelr init` or later with `labelr config`. Write clear descriptions for each label — the AI uses them to decide how to classify each email.

## How It Works

1. **Poll** — every 60 seconds, labelr checks Gmail for new messages
2. **Classify** — it sends the sender, subject, and a short body preview to your AI provider
3. **Label** — the AI picks the best-matching label, and labelr applies it in Gmail

Everything is queued locally in a SQLite database, so nothing gets lost if your computer goes to sleep or loses internet. When it comes back, it catches right up.

## Data & Privacy

All data stays on your machine in `~/.labelr/`:

- **Config** — your settings and label definitions
- **Credentials** — Gmail OAuth tokens (stored with restricted permissions)
- **Database** — local queue of processed messages
- **Logs** — daemon activity logs (auto-rotated)

The only external calls are to Gmail (to read/label emails) and your chosen AI provider (to classify them). If you use Ollama, everything stays completely local.

## Reliability

labelr is designed to handle real-world conditions:

- **Computer sleeps or reboots** — the service auto-restarts and catches up via Gmail's history API
- **Internet drops** — retries on the next poll cycle
- **AI provider is down** — retries up to 3 times before marking an email as failed
- **Crash mid-processing** — resets incomplete work on restart, nothing gets stuck

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
./bin/labelr init
```

Requires Go 1.25+.

## License

MIT
