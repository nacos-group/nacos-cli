#!/usr/bin/env bash
set -euo pipefail

SOURCE_REPO="${SOURCE_REPO:-$HOME/.nacos-cli/skill-repo}"
STATE_FILE="${STATE_FILE:-$HOME/.nacos-cli/skill-sync-state.json}"
DRY_RUN=false
OVERWRITE=false

usage() {
  cat <<'USAGE'
Restore legacy global skill-repo skills into agent skill directories.

Usage:
  scripts/restore-global-skill-repo-to-agents.sh [--dry-run] [--overwrite]

Environment:
  SOURCE_REPO   Legacy repo path. Default: ~/.nacos-cli/skill-repo
  STATE_FILE    Legacy state path. Default: ~/.nacos-cli/skill-sync-state.json

Behavior:
  - Reads agents from the legacy state file when available.
  - Falls back to well-known agent directories that exist.
  - Copies every valid skill directory from SOURCE_REPO to each agent.
  - Replaces agent symlinks with real directory copies.
  - Keeps existing real directories unless --overwrite is passed.
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run)
      DRY_RUN=true
      shift
      ;;
    --overwrite)
      OVERWRITE=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

run() {
  if [[ "$DRY_RUN" == true ]]; then
    printf '[dry-run] %q' "$1"
    shift
    for arg in "$@"; do
      printf ' %q' "$arg"
    done
    printf '\n'
  else
    "$@"
  fi
}

expand_home() {
  local path="$1"
  if [[ "$path" == "~" ]]; then
    printf '%s\n' "$HOME"
  elif [[ "$path" == "~/"* ]]; then
    printf '%s/%s\n' "$HOME" "${path#~/}"
  else
    printf '%s\n' "$path"
  fi
}

list_agents_from_state() {
  [[ -f "$STATE_FILE" ]] || return 1
  python3 - "$STATE_FILE" <<'PY'
import json
import sys

path = sys.argv[1]
try:
    with open(path, "r", encoding="utf-8") as f:
        data = json.load(f)
except Exception:
    sys.exit(1)

agents = data.get("agents") or []
for agent in agents:
    name = agent.get("name")
    path = agent.get("path")
    if name and path:
        print(f"{name}\t{path}")
PY
}

list_known_agents() {
  cat <<EOF
codex	$HOME/.codex/skills
claude	$HOME/.claude/skills
qoder	$HOME/.qoder/skills
qoderwork	$HOME/.qoderwork/skills
cursor	$HOME/.cursor/skills
kiro	$HOME/.kiro/skills
lingma	$HOME/.lingma/skills
copaw	$HOME/.copaw/skill_pool
openclaw	$HOME/.openclaw/skills
agents	$HOME/.agents/skills
default	$HOME/.skills
EOF
}

list_agents() {
  local tmp
  tmp="$(mktemp)"
  if list_agents_from_state > "$tmp" && [[ -s "$tmp" ]]; then
    cat "$tmp"
  else
    list_known_agents | while IFS=$'\t' read -r name path; do
      [[ -d "$path" ]] && printf '%s\t%s\n' "$name" "$path"
    done
  fi
  rm -f "$tmp"
}

list_skills() {
  find "$SOURCE_REPO" -mindepth 1 -maxdepth 1 -type d ! -name '.*' -print | while IFS= read -r dir; do
    [[ -f "$dir/SKILL.md" ]] && basename "$dir"
  done | sort
}

copy_skill_to_agent() {
  local skill="$1"
  local src="$SOURCE_REPO/$skill"
  local agent_name="$2"
  local agent_path="$3"
  local dest="$agent_path/$skill"

  if [[ ! -d "$agent_path" ]]; then
    echo "  $agent_name	missing agent dir, skipped ($agent_path)"
    return
  fi

  if [[ -L "$dest" ]]; then
    echo "  $agent_name	replacing symlink -> real copy"
    run rm "$dest"
    run mkdir -p "$dest"
    run cp -R "$src/." "$dest/"
    return
  fi

  if [[ -e "$dest" ]]; then
    if [[ "$OVERWRITE" != true ]]; then
      echo "  $agent_name	real path exists, kept ($dest)"
      return
    fi
    local backup="$agent_path/.skill-sync-backup/${skill}-$(date -u +%Y%m%dT%H%M%SZ)"
    echo "  $agent_name	backing up existing -> copy ($backup)"
    run mkdir -p "$(dirname "$backup")"
    run mv "$dest" "$backup"
    run mkdir -p "$dest"
    run cp -R "$src/." "$dest/"
    return
  fi

  echo "  $agent_name	new real copy"
  run mkdir -p "$dest"
  run cp -R "$src/." "$dest/"
}

if [[ ! -d "$SOURCE_REPO" ]]; then
  echo "Legacy skill repo not found: $SOURCE_REPO" >&2
  exit 1
fi

skills=()
while IFS= read -r skill_name; do
  skills+=("$skill_name")
done < <(list_skills)
if [[ ${#skills[@]} -eq 0 ]]; then
  echo "No valid skills found in $SOURCE_REPO" >&2
  exit 1
fi

agents=()
while IFS= read -r agent_line; do
  agents+=("$agent_line")
done < <(list_agents)
if [[ ${#agents[@]} -eq 0 ]]; then
  echo "No agent directories found from $STATE_FILE or well-known paths." >&2
  exit 1
fi

echo "Source repo: $SOURCE_REPO"
echo "State file:  $STATE_FILE"
echo "Skills:      ${#skills[@]}"
echo "Agents:      ${#agents[@]}"
echo

for skill_name in "${skills[@]}"; do
  echo "Restoring $skill_name"
  for agent_line in "${agents[@]}"; do
    IFS=$'\t' read -r agent_name agent_path <<< "$agent_line"
    agent_path="$(expand_home "$agent_path")"
    copy_skill_to_agent "$skill_name" "$agent_name" "$agent_path"
  done
  echo
done
