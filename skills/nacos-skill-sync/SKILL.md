---
version: 0.0.4
name: nacos-skill-sync
description: Use when Codex needs to synchronize local AI agent skills across Codex, Claude, Qoder, QoderWork, Cursor, Kiro, Lingma, CoPaw, OpenClaw, ~/.agents/skills, ~/.skills, and optionally a Nacos Skill Registry. Handles skill-sync setup, add/start/status/resolve/stop flows, Nacos mode and local mode, non-interactive agent execution, conflict triage, auto-upload state, Upload blocked handling, and safe human decision points.
---

# Nacos Skill Sync

Use this skill to operate `nacos-cli skill-sync` for the user. The default goal is to help the user run the first sync with the fewest decisions: check status, add the requested skill, start sync, verify status. The agent should do the mechanical work; the user should only make product decisions such as which source version is authoritative, whether to use Nacos, or whether to clear/review a remote draft.

## Core Rules

- Run `skill-sync` commands non-interactively by default.
- Never choose a local source arbitrarily when multiple different local versions exist. Ask the user which source to use, then rerun with `--from <agent>` or `--use-agent <agent>`.
- Never use `--use-remote-on-conflict`, `--refresh`, delete state, or clean backups without explicit user approval.
- Treat `status` as the source of truth after each operation.
- Prefer preserving data: conflicts should be recorded or resolved explicitly, not overwritten.
- If the user wants Nacos mode, use an explicit `--profile <name>` when known. If the profile is unknown, follow "Profile Check" before running Nacos-mode write operations.
- If no Nacos profile is available and the user did not require Nacos, local mode is acceptable.

## Pick the CLI

Until the published `npx` entrypoint is available, use this local binary path:

```bash
CLI=/Users/test/dev/nacos-cli/build/nacos-cli
$CLI skill-sync status
```

Use this `$CLI` for every command in this skill. If the command is missing or too old to support `--non-interactive`, tell the user to rebuild the current `nacos-cli` before continuing. When the npm package is published, replace this section with the `npx` invocation.

In examples below, `$CLI` means `/Users/test/dev/nacos-cli/build/nacos-cli`.

## Profile Check

For Nacos mode, inspect profiles before write operations:

```bash
$CLI profile list
$CLI profile show <profile>
```

If the user already gave a profile name, pass it explicitly on every Nacos-mode command:

```bash
$CLI skill-sync status --profile <profile>
```

If a profile needs to be created or corrected and the user has provided the connection values, use `profile set`:

```bash
$CLI profile set <profile> host=<host> port=<port> namespace=<namespace> auth-type=<auth-type>
```

Add auth fields only when the user provides them or they are already available in the environment/config:

```bash
$CLI profile set <profile> username=<username> password=<password>
$CLI profile set <profile> access-key=<access-key> secret-key=<secret-key>
```

Do not invent credentials. If required values are missing, ask the user to provide or configure them. Use `profile switch <profile>` only when the user wants that profile to become the default; otherwise keep using explicit `--profile <profile>`.

## First Check

Start every task with a read-only status check:

```bash
$CLI skill-sync status
```

If the user specified a Nacos profile:

```bash
$CLI skill-sync status --profile <profile>
```

Read these fields:

- `Mode`: `nacos` or `local`.
- `Profile`: required for Nacos mode.
- `Showing profile: <name> (inactive)`: this is a saved profile view only; it is not currently linked into agent directories.
- `Auto-upload`: whether local changes will upload automatically.
- `Sync daemon`: whether polling is running.
- `STATUS` and `NEXT`: the next required action for each skill.
- `AGENTS`: which agent directories are linked; a `!=` or mismatch marker means that agent differs from the central repo.

## Auto-Discovered Agents

`skill-sync` automatically discovers existing directories for:

| Agent | Default directory |
| --- | --- |
| `codex` | `~/.codex/skills` |
| `claude` | `~/.claude/skills` |
| `qoder` | `~/.qoder/skills` |
| `qoderwork` | `~/.qoderwork/skills` |
| `cursor` | `~/.cursor/skills` |
| `kiro` | `~/.kiro/skills` |
| `lingma` | `~/.lingma/skills` |
| `copaw` | `~/.copaw/skill_pool` |
| `openclaw` | `~/.openclaw/skills` |
| `agents` | `~/.agents/skills` |
| `default` | `~/.skills` |

Only directories that already exist are added automatically. If a user uses another directory, register it with `skill-sync agent add`.

## Add a Skill

### Nacos mode

If the user wants the team/Nacos version and provided a profile:

```bash
$CLI skill-sync add <skill> --profile <profile> --non-interactive
```

This defaults to the Nacos version when Nacos has the skill. If it succeeds, run:

```bash
$CLI skill-sync start --profile <profile> --non-interactive
$CLI skill-sync status --profile <profile>
```

If the user says a local agent version should win:

```bash
$CLI skill-sync add <skill> --profile <profile> --from <agent> --non-interactive
```

Use `--from latest` only when the user accepts "most recently modified local source" as the rule.

To add every not-yet-managed skill from Nacos for the current profile:

```bash
$CLI skill-sync add --all --profile <profile> --non-interactive
```

This should not be used to refresh existing local changes; use `start` or `resolve` for already managed skills.

### Local mode

If the user only wants local multi-agent sync:

```bash
$CLI skill-sync add <skill> --non-interactive
```

If this fails with multiple local sources, show the available agents from the error/status output and ask:

```text
Which local version should be the shared source: codex, claude, qoder, agents, default, or latest modified?
```

Then rerun:

```bash
$CLI skill-sync add <skill> --from <agent> --non-interactive
```

or:

```bash
$CLI skill-sync add <skill> --from latest --non-interactive
```

In local mode, a successful `add --from ...` should import the chosen source into the central repo and link every configured agent to it.

To add every skill currently in the local repo:

```bash
$CLI skill-sync add --all --non-interactive
```

In local mode, `add --all` may reverse-import unmanaged local agent skills when they are unambiguous. Multi-version conflicts still require a specific `add <skill> --from <agent>` decision.

## Start or Restart Sync

Use foreground only for debugging. In Nacos mode, start the daemon:

```bash
$CLI skill-sync start --profile <profile> --non-interactive
```

In local mode, `start` only links repo skills into agent directories. It does not start a background daemon because symlinks keep agents pointed at the central repo:

```bash
$CLI skill-sync start --non-interactive
```

If auto-upload should be disabled:

```bash
$CLI skill-sync start --profile <profile> --non-interactive --no-auto-upload
```

After Nacos-mode start, always verify:

```bash
$CLI skill-sync status --profile <profile>
```

After local-mode start, verify without a profile:

```bash
$CLI skill-sync status
```

If a Nacos-mode daemon is already running, do not kill it unless the user asked to restart. For restart:

```bash
$CLI skill-sync stop
$CLI skill-sync start --profile <profile> --non-interactive
```

## Resolve Conflicts

When `status` shows `Conflict`, do not guess. Ask the user to choose one of the valid source policies.

In Nacos mode, keep the same interaction model as `add`:

- use Nacos version;
- use one agent version, record `Local changes`, and let auto-upload decide whether to upload;
- exit/skip.

For Nacos as source of truth:

```bash
$CLI skill-sync resolve <skill> --use-nacos --non-interactive
```

For a local agent as source of truth:

```bash
$CLI skill-sync resolve <skill> --use-agent <agent> --non-interactive
```

In local mode, choose between the central repo version and one agent version:

```bash
$CLI skill-sync resolve <skill> --use-repo --non-interactive
$CLI skill-sync resolve <skill> --use-agent <agent> --non-interactive
```

For all conflicted skills only when the user explicitly wants the same policy for all:

```bash
$CLI skill-sync resolve --all --use-nacos --non-interactive
```

or:

```bash
$CLI skill-sync resolve --all --use-repo --non-interactive
```

or:

```bash
$CLI skill-sync resolve --all --use-agent <agent> --non-interactive
```

Then verify with `--profile <profile>` in Nacos mode, or without it in local mode:

```bash
$CLI skill-sync status --profile <profile>
$CLI skill-sync status
```

## Interpret States

- `Synced`: Nacos mode done.
- `Linked`: local mode done.
- `Local changes`: local content intentionally differs from Nacos. If auto-upload is enabled, wait for daemon upload; otherwise tell the user it stays local until manual upload.
- `Uploaded`: draft uploaded; wait for review/release. Keep checking status.
- `Upload blocked`: Nacos already has a draft or reviewing version. Do not overwrite. Tell the user the current remote draft/reviewing version exists and ask whether they want to review, release, or clear it in Nacos. After it is handled, auto-upload will retry.
- `Conflict`: ask for source-of-truth decision and run `resolve`.

## Remote Missing or Deleted

When daemon polling reports a tracked Nacos skill as missing, interpret it by local state:

- `Synced`: treat the Nacos deletion as authoritative. `skill-sync` stops managing the skill, replaces agent symlinks with real local copies, and removes it from sync state. Do not re-add or upload it unless the user explicitly asks.
- `Local changes`: keep the central repo and agent links. This is usually a local add or a user-chosen local source that has not uploaded yet; auto-upload may still proceed.
- `Upload blocked`: keep the central repo and agent links. The daemon should keep checking the Nacos draft/review state and retry after the remote blocker is handled.
- `Uploaded`: keep watching the recorded uploaded version and md5. If that exact uploaded version is released, status should move to `Synced`; if it disappears or changes, follow status output and ask the user before taking destructive action.

## Manual Upload Path

If auto-upload is disabled or the user wants to upload immediately:

```bash
$CLI skill-upload ~/.nacos-cli/skill-repo/<skill> --profile <profile>
```

After manual upload, run:

```bash
$CLI skill-sync status --profile <profile>
```

Uploaded skills should move to `Uploaded`, then to `Synced` after the uploaded version is released and the daemon sees the matching md5.

## Add Custom Agent Directory

If a user says an agent directory is missing from `AGENTS`, register it:

```bash
$CLI skill-sync agent add <agent-name> <absolute-skill-dir>
$CLI skill-sync agent list
```

Then rerun `add`, `start`, or `status`.

## Safe Human Prompts

Ask concise questions only at these points:

- Nacos or local mode when the user has not stated the desired source.
- Nacos profile name when Nacos mode is required but no profile is known.
- Which local agent source to use when multiple local versions exist.
- Whether Nacos or a local agent should win a `Conflict`.
- Whether to review/release/clear an existing Nacos draft or reviewing version.
- Whether to restart a running daemon.
- Whether to delete state, backups, or skill directories.

Do not ask the user to run commands manually unless the command requires external UI work or credentials the agent cannot access.

## Completion Criteria

Finish with a short summary including:

- final mode and profile if any;
- skills changed;
- final `STATUS` for each changed skill (`Synced` in Nacos mode, `Linked` in local mode);
- daemon state;
- any remaining `NEXT` action.

## Optional Advanced Operations

Do not use these during the first-run path unless the user explicitly asks or the current task cannot continue without them:

```bash
$CLI skill-sync remove <skill>
$CLI skill-sync add --all --profile <profile> --non-interactive
$CLI skill-sync set-label <label> --profile <profile>
```

- `remove` stops syncing a skill and keeps local agent copies; confirm before removing.
- `add --all` adds not-yet-managed skills in bulk without choosing a local source arbitrarily.
- `set-label` changes the tracked Nacos label, commonly `latest`; verify with `status`.
