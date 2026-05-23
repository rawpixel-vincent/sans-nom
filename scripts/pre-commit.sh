#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

fail() {
	printf 'pre-commit: %s\n' "$*" >&2
	exit 1
}

run() {
	label="$1"
	shift
	printf '\n==> %s\n' "$label"
	"$@"
}

run_in() {
	dir="$1"
	label="$2"
	shift 2
	printf '\n==> %s\n' "$label"
	(
		cd "$dir"
		"$@"
	)
}

find_exe() {
	name="$1"
	value="$(command -v "$name" || true)"
	if [ -z "$value" ]; then
		fail "$name is not available in PATH. Activate the project environment before committing."
	fi
	printf '%s\n' "$value"
}

resolve_exe() {
	value="$1"
	name="$2"
	case "$value" in
		*/*)
			if [ ! -x "$value" ]; then
				fail "$name points to a non-executable path: $value"
			fi
			printf '%s\n' "$value"
			;;
		*)
			find_exe "$value"
			;;
	esac
}

load_node_env() {
	if [ -n "${NVM_BIN:-}" ] && [ -x "$NVM_BIN/node" ]; then
		return
	fi
	nvm_dir="${NVM_DIR:-$HOME/.nvm}"
	if [ -s "$nvm_dir/nvm.sh" ]; then
		# shellcheck disable=SC1090
		. "$nvm_dir/nvm.sh"
		if [ -f "$repo_root/admin/.nvmrc" ]; then
			nvm use --silent "$(cat "$repo_root/admin/.nvmrc")" >/dev/null
		fi
	fi
}

go_bin="$repo_root/.go-sdk/go/bin/go"
if [ ! -x "$go_bin" ]; then
	run "Install local Go SDK" "$repo_root/scripts/install-go.sh" --yes
fi
if [ ! -x "$go_bin" ]; then
	fail "local Go binary is missing: $go_bin"
fi

python_bin="$repo_root/bridge/.venv/bin/python"
pytest_bin="$repo_root/bridge/.venv/bin/pytest"
uv_bin=""
if [ ! -x "$python_bin" ] || [ ! -x "$pytest_bin" ]; then
	uv_bin="$(resolve_exe "${UV:-uv}" "UV")"
fi

load_node_env
node_bin="$(resolve_exe "${NODE:-node}" "NODE")"
case "$node_bin" in
	/usr/bin/*|/usr/local/bin/*|/opt/homebrew/bin/*)
		fail "refusing system Node at $node_bin. Activate nvm/asdf/mise or set NODE to the environment Node executable."
		;;
esac

svelte_check_bin="$repo_root/admin/node_modules/svelte-check/bin/svelte-check"
vite_bin="$repo_root/admin/node_modules/vite/bin/vite.js"
if [ ! -x "$svelte_check_bin" ] || [ ! -x "$vite_bin" ]; then
	fail "admin local dependencies are missing. Run npm --prefix admin install in the active Node environment."
fi

run "Go tests" "$go_bin" test ./...
if [ -x "$python_bin" ] && [ -x "$pytest_bin" ]; then
	run "Bridge compile" "$python_bin" -m compileall bridge/src
	run "Bridge tests" "$pytest_bin" bridge/tests -q
else
	run "Bridge compile" "$uv_bin" run --project bridge python -m compileall bridge/src
	run "Bridge tests" "$uv_bin" run --project bridge pytest bridge/tests -q
fi
run_in "$repo_root/admin" "Admin check" "$node_bin" "$svelte_check_bin" --tsconfig ./tsconfig.json
run_in "$repo_root/admin" "Admin build" "$node_bin" "$vite_bin" build
