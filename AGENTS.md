# 🗺️ iMessage Anywhere

A web UI for sending and receiving iMessages from any machine on my LAN or Tailnet, while my Mac is online.

This folder is its own project root inside the `ai-conversations` archive. Paths below are relative to `imessage-anywhere/`. `CLAUDE.md` is a symlink to this file.

## Resuming work

1. Read [plan.md](plan.md).
2. Read the newest entries in [progress.md](progress.md).
3. Check the working copy (`sl status`) and any live state against the last handoff before acting.

## Building

- `app.js` is generated and not committed: run `go generate ./...` once in a fresh checkout and after editing `app.ts` (it type-checks `app.ts` and writes `app.js`; `go build` and `go test` need the file). `scripts/install-launch-agent.sh` runs it before building.
- `go test ./...` runs the seam tests; `scripts/install-launch-agent.sh` builds, signs, and restarts the launchd agent that serves the page. The agent runs the repo's `imessage-anywhere` file through a symlink in `~/.local/bin`, so don't leave a plain `go build -o imessage-anywhere` there: unsigned, it loses Full Disk Access at the agent's next restart.

## Commits

Commit after each milestone, together with its `progress.md` entry. Other agents work in this archive at the same time, so commit named paths under `imessage-anywhere/` only (`sl add <paths>` then `sl commit <paths>`), never `--addremove`.

## Agent skills

### Issue tracker

No ticket system: specs and work slices live in `plan.md`, status in the append-only `progress.md`. See `docs/agents/issue-tracker.md`.

### Domain docs

Single-context: one `GLOSSARY.md` and `docs/adr/` in this folder. See `docs/agents/domain.md`.
