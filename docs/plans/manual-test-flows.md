Manual Test Flows for Release

Setup — First-Run Wizard

1. Happy path: Fresh install (no ~/.labelr/) → Gmail OAuth → pick provider → select model → validate → select labels → add custom label → finish → daemon starts
2. Gmail OAuth failure: Decline Google consent or close browser mid-flow → error shown. (No error was shown, the TUI Just shows spinner indefinitely.)
3. AI model fetch failure: Pick a provider with no connectivity → should fall back to text input
4. AI validation failure: Enter wrong API key → retry prompt → go back to provider step
5. Ollama path: Pick Ollama → no API key prompt, model fetched from local
6. Back navigation: Advance to step 3, press Esc to go back to step 2, change provider
7. Custom labels: Add a custom label, try a duplicate name → see error, add another, decline adding more
8. Quit mid-wizard: Press q at step 2 → exits cleanly, no partial config saved
9. Ctrl+C at any step: Should quit cleanly, close DB

Setup — Reconfigure

10. Change Gmail account: Pick Gmail → browser opens → re-auth → labels synced to new account → daemon restarts
11. Change provider (full): Pick AI → full change → new provider + model + API key → validate → saved
12. Change model only: Pick AI → model-only → select new model → validate → saved
13. API key reuse: Change to same provider → "Use existing API key?" confirm → Yes → validates with old key
14. API key reuse declined: Same flow → No → enter new key
15. Add label: Labels → Add → name + description → synced to Gmail
16. Remove labels: Labels → Remove → multi-select → removed from Gmail
17. Duplicate label name: Labels → Add → enter existing name → error, re-prompt
18. Change poll interval: Poll → enter "30" → saved, daemon restarts
19. Invalid poll interval: Enter "abc" → error, re-prompt
20. Esc from any subflow: Should return to menu without saving

Status Dashboard

21. Daemon running: Shows green dot, stats update every 5s, "polled X ago" ticks
22. Daemon stopped: Shows red dot, stats still shown from DB
23. With activity: Recent entries show subject, label, relative time
24. No activity yet: Shows "No activity yet" message
25. Press r: Forces immediate refresh
26. Terminal resize: Dashboard should reflow

Logs Viewer

27. Normal viewing: Opens, shows last 1000 lines, new lines appear in real-time
28. Scroll up: Arrow up → auto-scroll pauses, shows "paused" indicator
29. Resume: Press G → jumps to bottom, resumes auto-scroll
30. Filter cycle: Press f → ERROR only → f → WARN+ERROR → f → all
31. No log file: Should error before TUI with "no log file found"
32. Log rotation: If daemon rotates the file mid-view → should reconnect after 5s

Sync

33. Happy path: labelr sync --last 7d → fetches → confirms → queues → done
34. No new emails: All already processed → "No new emails. All N were already processed."
35. Empty range: No emails at all → "No emails found in this time range."
36. Decline queue: Select "No" at confirm → exits immediately
37. Minutes support: labelr sync --last 30m → should work
38. Invalid duration: labelr sync --last 7w → error before TUI
39. No config: labelr sync without setup → "loading config" error

Start / Stop

40. Start fresh: labelr start → installs plist + loads → "running in the background"
41. Start without setup: No config → "no config found — run 'labelr setup' first"
42. Stop running daemon: labelr stop → "daemon stopped"
43. Stop when not running: Should still succeed (launchctl unload on non-loaded is fine)

Uninstall

44. Full uninstall (delete data): Stops daemon → removes plist → confirm No on keep data → deletes ~/.labelr/ → removes binary
45. Keep data: Confirm Yes → ~/.labelr/ preserved
46. Ctrl+C at data prompt: Prints "Cancelled.", exits without deleting anything
47. Plist already gone: Should not error (the fix we just made)
48. Daemon not running: Service removal still works cleanly

Cross-Cutting

49. Ctrl+C in any TUI: Should quit cleanly from any view
50. Terminal resize in any TUI: Header/footer/content should reflow
51. No ~/.labelr/ directory: Setup wizard should create it, other commands should fail gracefully