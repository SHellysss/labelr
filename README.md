# labelr

Automatically organize your Gmail inbox using AI. labelr runs in the background, reads new emails, and applies the right label.

## Install

```bash
curl -sSL https://raw.githubusercontent.com/Pankaj3112/labelr/main/install.sh | sh
```

Or grab a binary from the [releases page](https://github.com/Pankaj3112/labelr/releases).

## Quick Start

```bash
labelr setup
```

The wizard connects your Gmail, picks an AI provider, sets up labels, and starts the background service. That's it.

Check service health and queue stats anytime:

```bash
labelr status
```

To label old emails:

```bash
labelr sync --last 7d
```

Supports `30m`, `12h`, `7d`, etc.

## Commands

```
labelr setup                 # First-time setup wizard
labelr start                 # Start the background service
labelr stop                  # Stop the service
labelr status                # Queue stats and service health
labelr logs                  # Live log output
labelr sync --last <period>  # Label past emails
labelr uninstall             # Remove everything
```

Setup starts the service for you. You only need `start` after a manual `stop`.

## AI Providers

| Provider | Cost | Privacy | Speed |
|----------|------|---------|-------|
| **Ollama** | Free | Local | Depends on hardware |
| **Groq** | Free tier | Cloud | Very fast |
| **DeepSeek** | Very cheap | Cloud | Fast |
| **OpenAI** | Pay per use | Cloud | Fast |
| **Custom** | Varies | Varies | Varies |

Custom supports any OpenAI-compatible API. Each email costs ~100-300 input tokens and ~10 output tokens.

## Default Labels

Action Required, Informational, Newsletter, Finance, Scheduling, Personal, Automated.

All customizable during setup. Write clear descriptions so the AI knows how to classify.

## Privacy

All data stays on your machine in `~/.labelr/`. External calls go only to Gmail and your AI provider. With Ollama, everything is fully local.

## Platform Support

macOS (launchd), Linux (systemd), Windows (Task Scheduler). Auto-starts on boot.

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
