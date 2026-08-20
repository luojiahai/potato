## Agent skills

### Issue tracker

Issues and PRDs live as GitHub issues in `luojiahai/potato`, managed via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Default canonical labels (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`) used as-is. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context layout — `CONTEXT.md` at the repo root. See `docs/agents/domain.md`.

## Comments and docs

Comments and docs describe the code as it is, never as it was. No changelog in the source: not what a thing used to be called, not what was tried before, not what was removed to get here, not which release changed it, and not why the change was made. Where the history exists to stop someone reinstating a mistake, state it as the standing rule rather than as the story of when it was tried and what went wrong. The same goes for tests: name what the code does, never the change that produced it. `git log` and the release notes hold the past, and a reader of a file should never have to carry it.
