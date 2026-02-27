# UX Overhaul Design

## Summary

Replace `labelr init` and `labelr config` with a single `labelr setup` command that adapts based on state. Fix all 18 identified UX issues. Make `labelr uninstall` a full uninstall.

## Command Surface

```
labelr setup       # smart wizard: first-time OR reconfigure
labelr start       # start daemon
labelr stop        # stop daemon
labelr status      # show status + queue stats
labelr logs        # tail daemon logs
labelr sync        # backlog scan
labelr uninstall   # full uninstall: service + binary, optionally data
```

Removed: `init`, `config` — both replaced by `setup`.

## `labelr setup` — First Run (No config exists)

Full wizard:

1. **Gmail OAuth** — opens browser directly (no "Ready?" confirmation). Shows fallback URL if browser doesn't open.
2. **AI Provider** — select from deterministic list: openai, deepseek, groq, ollama (fixed order, not random map iteration).
3. **Model** — fetch from models.dev (cloud) or `/api/tags` (ollama, all pulled models — not just running). Fallback to text input if fetch fails.
4. **API Key** — check env var first, then prompt. Password mode input.
5. **Validate connection** — spinner, test actual API call. Retry loop on failure.
6. **Labels** — multi-select defaults (all pre-selected), then loop for custom labels.
7. **Create labels in Gmail** — assign colors from palette, store color mapping in DB for stability.
8. **Test run** — offer to label 10 recent emails.
9. **Auto-start daemon** — install and start background service.

## `labelr setup` — Re-run (Config exists)

Shows current config summary, then menu:

```
Current configuration
──────────────────────
Gmail        user@example.com
Provider     openai
Model        gpt-4o-mini
Labels       Action Required, Informational, Newsletter,
             Finance, Scheduling, Personal, Automated, projects
Poll         60s

What would you like to change?
> Gmail account
  AI provider / model
  Just the model
  Labels
  Poll interval
  Nothing, exit
```

### Menu behaviors

- **Menu loops** — after each change, returns to menu so user can change multiple things.
- **Daemon restarts** after every change that affects behavior (Gmail, AI, labels, poll).

### Gmail account

Opens browser for OAuth. Updates email in config. No extra confirmation — user chose this option.

### AI provider / model

Full flow: select provider → fetch models → select model → API key → validate connection → save.

API key logic:
- Check env var for new provider first.
- If switching providers: always prompt for new key (don't offer to reuse old provider's key).
- If same provider and key exists: offer to reuse.

### Just the model

Skip provider selection and API key. Fetch models for current provider → select → validate → save.

### Labels

Show current labels as multi-select (all selected). User deselects to remove, keeps to retain.
- Deselected labels are removed from config AND from Gmail via API.
- Then loop for adding custom labels (same as first-run).
- New labels get colors assigned; existing labels keep their stored colors.
- Colors are stored in DB `label_mappings` table (extended with color columns).

### Poll interval

Prompt for seconds. Validate: must be a positive integer. Show error and re-prompt on bad input.

## `labelr uninstall` — Full Uninstall

```
$ labelr uninstall

→ Stopping daemon...
✓ Background service removed

Keep your data (~/.labelr/)? (y/n)
> y
→ Data kept at ~/.labelr/

✓ Binary removed
labelr has been uninstalled.
```

- Always: stop daemon, remove service, remove binary (`os.Executable()`).
- Ask: keep or delete `~/.labelr/` (config, credentials, DB, logs).

## Bug Fixes

### 1. Provider list order (was random)
Use a fixed slice `[]string{"openai", "deepseek", "groq", "ollama"}` instead of iterating `map[string]Provider`.

### 2. API key reuse logic (was broken)
Save `previousProvider` before mutating `cfg.AI.Provider`. Compare new selection against saved value.

### 3. Poll interval validation (was silent on bad input)
Parse input, validate > 0, show error message and re-prompt if invalid.

### 4. No AI validation in config (was missing)
Add the same "Verifying connection..." spinner + validation that exists in init.

### 5. Labels reset on re-init (was using defaults)
On re-run, start from current config labels, not `DefaultLabels()`.

### 6. Daemon not restarted after config changes (was silent)
After saving config, detect if daemon is running and restart it.

### 7. Ollama model listing (was running-only)
Switch from `/api/ps` to `/api/tags` to show all pulled models.

### 8. Label color stability
Store color assignments in DB. On re-run, existing labels keep colors. New labels get next unused color from palette.

### 9. Label removal not synced to Gmail
When labels are deselected in setup, delete them from Gmail via API (not just from config).

## Files to Modify

| File | Changes |
|------|---------|
| `cmd/labelr/main.go` | Replace init+config registration with setup |
| `internal/cli/init.go` | Rename to `setup.go`, rewrite as unified command |
| `internal/cli/config_cmd.go` | Delete (merged into setup) |
| `internal/cli/models.go` | Keep, fix Ollama to use `/api/tags` |
| `internal/cli/uninstall.go` | Add binary self-deletion |
| `internal/ai/providers.go` | Add `ProviderNamesOrdered()` returning fixed slice, fix Ollama endpoint |
| `internal/gmail/client.go` | Add `DeleteLabel()` method |
| `internal/gmail/colors.go` | Support stable color lookup from DB |
| `internal/db/db.go` | Add color columns to label_mappings |
| `internal/ui/style.go` | No changes |
