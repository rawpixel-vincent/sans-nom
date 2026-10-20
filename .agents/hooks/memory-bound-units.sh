#!/usr/bin/env bash
# Hand the model the memory units bound to the file it is about to change or, with
# MEMORY_HOOK_ON_READ=1, the file it just read. One script serves two harnesses:
#   Claude Code  PreToolUse on Write|Edit|MultiEdit  (PostToolUse on Read with the variable)
#   Cursor       preToolUse on Write|StrReplace      (postToolUse on Read with the variable)
# The rows come from `memory:find --at <file>` run in the checkout that holds the file.
# Silent for files outside a checkout, unit files, excluded trees, and files already
# announced in this session.
# Managed by shared-agent-memory (.agents/shared-memory.lock.json): change it upstream
# and run its setup; a local edit blocks the next upgrade of this file.
set -uo pipefail

input=$(cat || true)
event=$(printf '%s' "$input" | jq -r '.hook_event_name // empty' 2>/dev/null)
file=$(printf '%s' "$input" | jq -r '.tool_input.file_path // .tool_input.target_file // .tool_input.path // empty' 2>/dev/null)
session=$(printf '%s' "$input" | jq -r '.session_id // .conversation_id // "no-session"' 2>/dev/null)

silent() {
  echo '{}'
  exit 0
}

case "$event" in
  PreToolUse | preToolUse) ;;
  PostToolUse | postToolUse) [ "${MEMORY_HOOK_ON_READ:-}" = "1" ] || silent ;;
  *) silent ;;
esac
[ -n "$file" ] || silent

# A relative path is relative to the session's working directory.
case "$file" in
  /*) ;;
  *)
    base=$(printf '%s' "$input" | jq -r '.cwd // .workspace_roots[0] // empty' 2>/dev/null)
    [ -n "$base" ] || silent
    file="$base/$file"
    ;;
esac

# The checkout that holds the file: walk up to the store and the finder.
root=""
dir=$(dirname "$file")
while [ -n "$dir" ] && [ "$dir" != "/" ]; do
  if [ -f "$dir/.agents/README.md" ] && [ -f "$dir/scripts/agents-find.ts" ]; then
    root="$dir"
    break
  fi
  dir=$(dirname "$dir")
done
[ -n "$root" ] || silent
rel="${file#"$root/"}"
case "/$rel/" in
  /.agents/* | */node_modules/* | */dist/* | */build/*) silent ;;
esac

# Once per file per session.
state="${TMPDIR:-/tmp}/memory-hook/${session}"
mkdir -p "$state"
marker="$state/$(printf '%s' "$rel" | tr '/' '_')"
[ -e "$marker" ] && silent
: > "$marker"

rows=$(cd "$root" && bun scripts/agents-find.ts find --at "$rel" --limit 3 2>/dev/null)
case "$rows" in
  '' | 'no unit is bound'* | 'no match'*) silent ;;
esac
count=$(printf '%s\n' "$rows" | grep -c '^')
text="Memory units bound to ${rel} (facts the code cannot state; bun run memory:read <unit> prints one):"$'\n'"$rows"
case "$event" in
  PreToolUse | PostToolUse)
    jq -cn --arg ev "$event" --arg ctx "$text" '{hookSpecificOutput: {hookEventName: $ev, additionalContext: $ctx}}' ;;
  preToolUse)
    jq -cn --arg ctx "$text" '{permission: "allow", additional_context: $ctx}' ;;
  postToolUse)
    jq -cn --arg ctx "$text" '{additional_context: $ctx}' ;;
esac
printf '%s %s %s %s\n' "$(date +%s)" "$session" "$rel" "$count" >> "${TMPDIR:-/tmp}/memory-hook/log"
