# Claude Console

Single Go binary that reads local Claude Code JSONL logs (`~/.claude/projects/`) and prints a usage dashboard: totals, per-model spend, monthly cost, 7-day sparkline, top projects.

100% offline. No API calls.

## Build

```sh
go build -o ai-console-watch .
```

## Run

```sh
./ai-console-watch                 # one-shot snapshot
./ai-console-watch -watch          # refresh every 5s
./ai-console-watch -month          # current calendar month only
./ai-console-watch -month -watch
BUDGET=100 ./ai-console-watch -watch
CLAUDE_DIR=/custom/path ./ai-console-watch
```

Estimates use Anthropic list prices. On Pro/Max plans treat the dollar amounts as API-equivalent value, not billed API usage.
