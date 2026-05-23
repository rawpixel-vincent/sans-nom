#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git -C "$(dirname "$0")/.." rev-parse --show-toplevel)"
GO_VERSION="${GO_VERSION:-1.26.3}"
GO_TAR="${ROOT}/go.tar.gz"
GO_SDK_DIR="${ROOT}/.go-sdk"
GO_BIN="${GO_SDK_DIR}/go/bin/go"
ASSUME_YES=0

for arg in "$@"; do
	case "$arg" in
		-y|--yes)
			ASSUME_YES=1
			;;
		*)
			echo "Unsupported argument: $arg" >&2
			exit 1
			;;
	esac
done

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
	x86_64) arch=amd64 ;;
	aarch64|arm64) arch=arm64 ;;
	*)
		echo "Unsupported architecture: $(uname -m)" >&2
		exit 1
		;;
esac
case "$os" in
	linux) goos=linux ;;
	darwin) goos=darwin ;;
	*)
		echo "Unsupported OS: $os" >&2
		exit 1
		;;
esac

GO_URL="https://go.dev/dl/go${GO_VERSION}.${goos}-${arch}.tar.gz"

cleanup() {
	rm -f "$GO_TAR"
}
trap cleanup EXIT

if [ -x "$GO_BIN" ]; then
	installed="$("$GO_BIN" version | awk '{print $3}' | sed 's/^go//')"
	if [ "$installed" = "$GO_VERSION" ]; then
		echo "Go SDK already installed: $("$GO_BIN" version)"
		exit 0
	fi
	echo "Local Go SDK is $installed, expected $GO_VERSION"
	if [ "$ASSUME_YES" -ne 1 ]; then
		printf 'Reinstall local Go SDK? (y/n): '
		read -r answer
		if [ "$answer" != "y" ]; then
			exit 0
		fi
	fi
	rm -rf "$GO_SDK_DIR"
fi

echo "Installing Go SDK ${GO_VERSION}..."
curl -fsSL "$GO_URL" -o "$GO_TAR"
mkdir -p "$GO_SDK_DIR"
tar -C "$GO_SDK_DIR" -xzf "$GO_TAR"
chmod -R 755 "$GO_SDK_DIR"
chmod +x "$GO_BIN"
"$GO_BIN" version
echo "Go SDK installed successfully"
