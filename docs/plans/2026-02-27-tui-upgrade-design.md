# Labelr TUI Upgrade Design

**Date:** 2026-02-27
**Status:** Approved
**Scope:** Upgrade all major CLI commands from sequential prompts to a unified Bubble Tea TUI experience.

## Overview

Replace labelr's current print-and-exit / sequential-prompt CLI with a polished Bubble Tea TUI. All interactive commands share a consistent app shell (header, content area, help footer). Four commands get TUI upgrades: `setup`, `status`, `logs`, `sync`. Three commands stay simple: `start`, `stop`, `uninstall`.

## Architecture: Unified App Shell

Every TUI command uses a shared `ShellModel` that provides consistent framing:

```
┌─────────────────────────────────────────────┐
│  ● labelr <command>                         │  ← header
├─────────────────────────────────────────────┤
│                                             │
│  [Content area — command-specific view]     │
│                                             │
├─────────────────────────────────────────────┤
│  ↑/↓ navigate • enter select • esc back     │  ← footer (context-sensitive help)
└─────────────────────────────────────────────┘
```

### View Interface

Each command implements a `View` interface consumed by the shell:

```go
type View interface {
    Init() tea.Cmd
    Update(msg tea.Msg) (View, tea.Cmd)
    View() string
    HelpKeys() []key.Binding  // context-sensitive key bindings for footer
    Title() string            // command name for header
}
```

The shell:
- Renders the header with branded styling (green bullet + "labelr" + command name)
- Delegates `Update`/`View` to the inner view
- Renders the footer by calling `HelpKeys()` on the active view
- Handles global keys: `Ctrl+C` to quit

## Command 1: Setup Wizard

### Layout

```
┌─────────────────────────────────────────────┐
│  ● labelr setup                             │
├─────────────────────────────────────────────┤
│  Step 2 of 5 · AI Provider                  │
│  ━━━━━━━━━━━━━━━━━━━━━░░░░░░░░░░░░░░░░░░░░  │
│                                             │
│  Choose your AI provider:                   │
│    ● OpenAI                                 │
│      DeepSeek                               │
│      Groq                                   │
│      Ollama                                 │
│                                             │
├─────────────────────────────────────────────┤
│  ↑/↓ navigate • enter select • shift+tab back│
└─────────────────────────────────────────────┘
```

### Steps (First-Time Setup)

| # | Step | Fields | Async? |
|---|------|--------|--------|
| 1 | Gmail | OAuth flow — opens browser, spinner while waiting for callback | Yes |
| 2 | AI Provider | Select provider, select/input model, input API key | Partially (model fetch is async) |
| 3 | Validate | Spinner — validate AI connection. Retry on failure | Yes |
| 4 | Labels | MultiSelect defaults + loop to add custom labels | No |
| 5 | Finish | Create labels (spinner), offer test run, auto-start daemon | Yes |

### Behaviors

- **Step indicator**: "Step N of M" with a lipgloss progress bar
- **Back navigation**: `Shift+Tab` / `Esc` goes to previous step (except can't go back past Gmail OAuth completion)
- **Async steps**: Wizard shows a spinner view during async work, auto-advances on success, shows retry option on failure
- **Reconfigure mode**: Instead of the step wizard, renders a menu (like today) styled within the shell

### Files

- `internal/tui/setup/wizard.go` — wizard model managing step state, progress bar, navigation
- `internal/tui/setup/steps.go` — individual step models (gmail, ai, labels, finish), each implementing a `Step` interface with `CanGoBack() bool`

## Command 2: Status Dashboard

### Layout

```
┌─────────────────────────────────────────────┐
│  ● labelr status                            │
├─────────────────────────────────────────────┤
│  Daemon    ● Running                        │
│  Gmail     pankaj@gmail.com                 │
│  Provider  openai / gpt-4o-mini             │
│  Polling   every 60s                        │
│                                             │
│  ┌─────────┬──────────┬─────────┐           │
│  │ Pending  │ Labeled  │ Failed  │           │
│  │   12     │   847    │    3    │           │
│  └─────────┴──────────┴─────────┘           │
│                                             │
│  Last poll: 23s ago   Refreshes: 5s         │
│                                             │
├─────────────────────────────────────────────┤
│  q quit • r refresh now                     │
└─────────────────────────────────────────────┘
```

### Behaviors

- **Auto-refresh**: Polls database stats every 5 seconds via `tea.Tick`
- **Daemon health**: Checks if daemon is running on each refresh
- **Stats cards**: Pending (yellow), Labeled (green), Failed (red) in bordered boxes via lipgloss
- **Live timer**: "Last poll: Xs ago" updates every second
- **Keys**: `q` quits, `r` forces immediate refresh

### Files

- `internal/tui/status/dashboard.go` — dashboard model, reads from database and service manager

## Command 3: Logs Viewer

### Layout

```
┌─────────────────────────────────────────────┐
│  ● labelr logs                              │
├─────────────────────────────────────────────┤
│  14:23:01  INFO   Polling for new emails    │
│  14:23:02  INFO   Found 3 new messages      │
│  14:23:03  INFO   Labeling msg abc123       │
│  14:23:03  INFO   → Applied label: Work     │
│  14:23:04  WARN   Rate limited, retrying    │
│  14:23:06  ERROR  Failed to classify msg    │
│  14:23:07  INFO   → Applied label: Personal │
│  14:23:10  INFO   Poll complete, sleeping   │
│                                             │
├─────────────────────────────────────────────┤
│  ↑/↓ scroll • f filter • / search • q quit │
└─────────────────────────────────────────────┘
```

### Behaviors

- **Colored log levels**: INFO (default), WARN (yellow), ERROR (red)
- **Live tailing**: New lines appear at bottom, auto-scrolls
- **Scroll**: Arrow keys / PgUp/PgDn to scroll history (pauses auto-scroll)
- **Filter**: `f` toggles level filter (e.g., WARN+ERROR only)
- **Search**: `/` opens search input, highlights matches
- **Resume**: `G` jumps to bottom and resumes auto-scroll
- **File watching**: Periodic polling (500ms) or fsnotify for new log data

### Files

- `internal/tui/logs/viewer.go` — log viewer model using `bubbles/viewport`
- `internal/tui/logs/parser.go` — parse log lines into structured entries (timestamp, level, message)

## Command 4: Sync Progress

### Layout (Fetching Phase)

```
┌─────────────────────────────────────────────┐
│  ● labelr sync                              │
├─────────────────────────────────────────────┤
│  Fetching emails from the last 7 days...    │
│  ━━━━━━━━━━━━━━━━━━━━━░░░░░░░░░░░░░░░░░░░░  │
│  Found 142 emails                           │
│                                             │
│  Queue these 142 emails for labeling?       │
│    ● Yes                                    │
│      No                                     │
│                                             │
├─────────────────────────────────────────────┤
│  enter select • q quit                      │
└─────────────────────────────────────────────┘
```

### Layout (Queuing Phase)

```
│  Queuing emails for labeling...             │
│  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━  │
│  ✓ 142/142 emails queued                    │
│                                             │
│  Done! The daemon will process these.       │
```

### Behaviors

- **Pulsing progress bar** during fetch (indeterminate — total unknown)
- **Confirmation prompt** styled within the shell
- **Determinate progress bar** during queuing (total known)
- **Auto-exits** after completion with brief success display

### Files

- `internal/tui/sync/view.go` — sync model with states: fetch → confirm → queue → done
- Uses `bubbles/progress` for progress bar

## Commands That Stay Simple

These remain print-and-exit with current `ui.*` styling:

- **`labelr start`** — "Starting daemon..." + success/error
- **`labelr stop`** — "Stopping daemon..." + success/error
- **`labelr uninstall`** — confirmation prompt (add `WithShowHelp(true)`) + removal steps

## File Organization

```
internal/tui/
├── shell.go          # Shared ShellModel (header, footer, View interface)
├── keys.go           # Shared key binding definitions
├── theme.go          # Shared lipgloss styles/colors (extends ui/style.go palette)
├── setup/
│   ├── wizard.go     # Setup wizard model (step management, progress bar, nav)
│   └── steps.go      # Individual step models (gmail, ai, labels, finish)
├── status/
│   └── dashboard.go  # Live status dashboard
├── logs/
│   ├── viewer.go     # Log viewer model
│   └── parser.go     # Log line parser
└── sync/
    └── view.go       # Sync progress view
```

Existing `internal/ui/style.go` stays for non-TUI commands. New `internal/tui/theme.go` extends the same green/red/yellow color palette with lipgloss styles for borders, cards, progress bars.

## Dependencies

All already in `go.mod`:
- `charmbracelet/bubbletea v1.3.10` — core TUI framework
- `charmbracelet/bubbles v1.0.0` — viewport, progress, spinner, help components
- `charmbracelet/lipgloss v1.1.0` — styling and layout
- `charmbracelet/huh v0.8.0` — may still be used for embedded form fields within Bubble Tea models

No new dependencies required.

## Migration Strategy

Each command is migrated independently. The CLI entry point (`main.go`) switches from calling the current function to launching the Bubble Tea program. Old command code can be removed once the TUI replacement is verified working.

Order of implementation:
1. Shell (shared infrastructure)
2. Setup wizard (most complex, most impactful)
3. Status dashboard (high user value, moderate complexity)
4. Logs viewer (moderate complexity)
5. Sync progress (simplest TUI upgrade)
