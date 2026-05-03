#!/usr/bin/env sh
set -eu

command -v go >/dev/null 2>&1 || {
  echo "go is required" >&2
  exit 1
}

command -v uv >/dev/null 2>&1 || {
  echo "uv is required" >&2
  exit 1
}

command -v claude >/dev/null 2>&1 || {
  echo "claude CLI is required for Phase 0 writer/narrator calls" >&2
  exit 1
}

go test ./...
python3 -m compileall bridge/src

echo "Phase 0 scaffold verified."

