.PHONY: test cli sidecar dev lore-manifest lore-extract-dry-run lore-extract-volume-1 lore-validate lore-ingest

test:
	go test ./...
	python3 -m compileall bridge/src
	uv run --project bridge pytest bridge/tests -q

cli:
	go run ./cmd/cli

sidecar:
	uv run --project bridge uvicorn bridge.main:app --host 127.0.0.1 --port 8787

dev:
	@echo "Run 'make sidecar' in one terminal and 'make cli' in another."

lore-manifest:
	uv run --project bridge comic-lore --root . manifest

lore-extract-dry-run:
	uv run --project bridge comic-lore --root . extract-pages --limit 3 --dry-run

lore-extract-volume-1:
	uv run --project bridge comic-lore --root . extract-pages --volume 1 --workers 4
	uv run --project bridge comic-lore --root . reconcile-volumes --volume 1
	uv run --project bridge comic-lore --root . extract-characters --volume 1

lore-validate:
	uv run --project bridge comic-lore --root . validate --strict

lore-ingest:
	uv run --project bridge comic-lore --root . ingest-qdrant
