from __future__ import annotations

import argparse
import concurrent.futures
import hashlib
import json
import os
import re
import subprocess
import sys
import tempfile
from collections import defaultdict
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Iterable

from pydantic import BaseModel, Field, ValidationError

from .env import load_project_env

load_project_env()

IMAGE_RE = re.compile(
    r"^(?P<ordinal>\d{3})_The_Metabarons_(?P<volume>\d+)_(?P<label>[^.]+)\.(?P<ext>jpe?g|png)$",
    re.IGNORECASE,
)

DEFAULT_IMAGE_DIR = Path("external/comics/meta-barons")
DEFAULT_OUTPUT_DIR = Path("lore/packs/metabarons/comics")
DEFAULT_COLLECTION = "sans_nom_lore_metabarons"


class PageRef(BaseModel):
    page_id: str
    source_image: str
    image_sha256: str


class PageSummary(BaseModel):
    page_id: str
    volume: int
    page_label: str
    is_spread: bool
    source_image: str
    image_sha256: str
    confidence: str = Field(pattern="^(high|medium|low)$")
    visual_summary: str
    plot_events: list[str] = Field(default_factory=list)
    characters_present: list[str] = Field(default_factory=list)
    locations: list[str] = Field(default_factory=list)
    dialogue_meaning: list[str] = Field(default_factory=list)
    lore_facts: list[str] = Field(default_factory=list)
    character_facts: list[str] = Field(default_factory=list)
    continuity_links: list[str] = Field(default_factory=list)
    uncertainties: list[str] = Field(default_factory=list)
    source_refs: list[PageRef] = Field(default_factory=list)


class VolumeSummary(BaseModel):
    volume: int
    title: str
    page_ids: list[str]
    source_images: list[str]
    chronology: list[str] = Field(default_factory=list)
    major_events: list[str] = Field(default_factory=list)
    recurring_motifs: list[str] = Field(default_factory=list)
    lore_laws: list[str] = Field(default_factory=list)
    character_arcs: dict[str, list[str]] = Field(default_factory=dict)
    locations: list[str] = Field(default_factory=list)
    open_uncertainties: list[str] = Field(default_factory=list)


class CharacterLore(BaseModel):
    character_id: str
    peer_candidate: bool = True
    non_peer_reason: str = ""
    related_peer_ids: list[str] = Field(default_factory=list)
    aliases: list[str] = Field(default_factory=list)
    volumes: list[int] = Field(default_factory=list)
    appearances: list[str] = Field(default_factory=list)
    traits: list[str] = Field(default_factory=list)
    relationships: list[str] = Field(default_factory=list)
    witnessed_facts: list[str] = Field(default_factory=list)
    knowledge_boundaries: list[str] = Field(default_factory=list)
    important_actions: list[str] = Field(default_factory=list)
    voice_notes: list[str] = Field(default_factory=list)
    source_refs: list[PageRef] = Field(default_factory=list)


class ManifestEntry(BaseModel):
    page_id: str
    ordinal: int
    volume: int
    page_label: str
    is_spread: bool
    source_image: str
    image_sha256: str
    byte_size: int


class Manifest(BaseModel):
    image_dir: str
    page_count: int
    entries: list[ManifestEntry]


@dataclass(frozen=True)
class Paths:
    root: Path
    output_dir: Path

    @property
    def manifest(self) -> Path:
        return self.output_dir / "manifest.json"

    @property
    def pages(self) -> Path:
        return self.output_dir / "pages"

    @property
    def volumes(self) -> Path:
        return self.output_dir / "volumes"

    @property
    def characters(self) -> Path:
        return self.output_dir / "characters"

    @property
    def review_needed(self) -> Path:
        return self.output_dir / "review-needed.jsonl"


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="comic-lore", description="Extract Metabarons comic lore.")
    parser.add_argument("--root", default=".", help="Repository root.")
    parser.add_argument("--image-dir", default=str(DEFAULT_IMAGE_DIR), help="Comic image directory.")
    parser.add_argument("--output-dir", default=str(DEFAULT_OUTPUT_DIR), help="Lore output directory.")
    sub = parser.add_subparsers(dest="cmd", required=True)

    sub.add_parser("manifest", help="Build image manifest.")

    extract = sub.add_parser("extract-pages", help="Run Codex vision page workers.")
    extract.add_argument("--limit", type=int, default=0, help="Maximum pages to process.")
    extract.add_argument("--volume", type=int, default=0, help="Restrict to one volume.")
    extract.add_argument("--workers", type=int, default=4, help="Concurrent page workers.")
    extract.add_argument("--force", action="store_true", help="Regenerate even if hash matches.")
    extract.add_argument("--dry-run", action="store_true", help="Print selected pages without running Codex.")
    extract.add_argument("--codex-model", default=os.getenv("SANS_NOM_CODEX_VISION_MODEL", "gpt-5.4"))
    extract.add_argument("--codex-effort", default=os.getenv("SANS_NOM_CODEX_VISION_EFFORT", "medium"))

    reconcile = sub.add_parser("reconcile-volumes", help="Use Claude Sonnet to reconcile page summaries by volume.")
    reconcile.add_argument("--volume", type=int, default=0, help="Restrict to one volume.")
    reconcile.add_argument("--force", action="store_true")

    chars = sub.add_parser("extract-characters", help="Use Claude Sonnet to extract structured character lore.")
    chars.add_argument("--volume", type=int, default=0, help="Restrict source summaries to one volume.")
    chars.add_argument("--force", action="store_true")

    ingest = sub.add_parser("ingest-qdrant", help="Embed generated lore and upsert to Qdrant.")
    ingest.add_argument("--collection", default=DEFAULT_COLLECTION)
    ingest.add_argument("--dry-run", action="store_true")

    smoke = sub.add_parser("smoke-search", help="Run retrieval smoke queries against Qdrant.")
    smoke.add_argument("--collection", default=DEFAULT_COLLECTION)

    validate = sub.add_parser("validate", help="Validate generated JSON files.")
    validate.add_argument("--strict", action="store_true")

    args = parser.parse_args(argv)
    root = Path(args.root).resolve()
    paths = Paths(root=root, output_dir=(root / args.output_dir))
    image_dir = root / args.image_dir

    if args.cmd == "manifest":
        manifest = build_manifest(image_dir, root=root)
        write_json_atomic(paths.manifest, manifest.model_dump())
        print(f"wrote {paths.manifest} ({manifest.page_count} pages)")
        return 0
    if args.cmd == "extract-pages":
        manifest = load_or_build_manifest(paths, image_dir)
        selected = select_entries(manifest.entries, volume=args.volume, limit=args.limit)
        if args.dry_run:
            for entry in selected:
                print(entry.page_id, entry.source_image)
            return 0
        failures = extract_pages(
            root=root,
            paths=paths,
            entries=selected,
            workers=args.workers,
            force=args.force,
            codex_model=args.codex_model,
            codex_effort=args.codex_effort,
        )
        return 1 if failures else 0
    if args.cmd == "reconcile-volumes":
        reconcile_volumes(paths, volume=args.volume, force=args.force)
        return 0
    if args.cmd == "extract-characters":
        extract_characters(paths, volume=args.volume, force=args.force)
        return 0
    if args.cmd == "ingest-qdrant":
        ingest_qdrant(paths, collection=args.collection, dry_run=args.dry_run)
        return 0
    if args.cmd == "smoke-search":
        smoke_search(root=root, collection=args.collection)
        return 0
    if args.cmd == "validate":
        validate_outputs(paths, strict=args.strict)
        return 0
    raise AssertionError(args.cmd)


def build_manifest(image_dir: Path, root: Path | None = None) -> Manifest:
    entries: list[ManifestEntry] = []
    for path in sorted(image_dir.iterdir()):
        if not path.is_file():
            continue
        match = IMAGE_RE.match(path.name)
        if not match:
            continue
        label = match.group("label")
        source_image = path.as_posix()
        if root is not None:
            try:
                source_image = path.resolve().relative_to(root.resolve()).as_posix()
            except ValueError:
                source_image = path.as_posix()
        entry = ManifestEntry(
            page_id=path.stem,
            ordinal=int(match.group("ordinal")),
            volume=int(match.group("volume")),
            page_label=label,
            is_spread="-p" in label and "-" in label,
            source_image=source_image,
            image_sha256=sha256_file(path),
            byte_size=path.stat().st_size,
        )
        entries.append(entry)
    image_dir_value = image_dir.as_posix()
    if root is not None:
        try:
            image_dir_value = image_dir.resolve().relative_to(root.resolve()).as_posix()
        except ValueError:
            image_dir_value = image_dir.as_posix()
    return Manifest(image_dir=image_dir_value, page_count=len(entries), entries=entries)


def load_or_build_manifest(paths: Paths, image_dir: Path) -> Manifest:
    if paths.manifest.exists():
        return Manifest.model_validate_json(paths.manifest.read_text())
    manifest = build_manifest(image_dir, root=paths.root)
    write_json_atomic(paths.manifest, manifest.model_dump())
    return manifest


def select_entries(entries: list[ManifestEntry], volume: int = 0, limit: int = 0) -> list[ManifestEntry]:
    selected = [entry for entry in entries if volume == 0 or entry.volume == volume]
    if limit:
        selected = selected[:limit]
    return selected


def extract_pages(
    root: Path,
    paths: Paths,
    entries: list[ManifestEntry],
    workers: int,
    force: bool,
    codex_model: str,
    codex_effort: str,
) -> int:
    paths.pages.mkdir(parents=True, exist_ok=True)
    failures = 0
    with concurrent.futures.ThreadPoolExecutor(max_workers=max(1, workers)) as pool:
        futures = [
            pool.submit(extract_one_page, root, paths, entry, force, codex_model, codex_effort)
            for entry in entries
        ]
        for future in concurrent.futures.as_completed(futures):
            try:
                result = future.result()
            except Exception as exc:  # noqa: BLE001 - log and continue batch.
                failures += 1
                append_jsonl(paths.review_needed, {"error": str(exc)})
                print(f"FAILED {exc}", file=sys.stderr)
            else:
                print(result)
    return failures


def extract_one_page(
    root: Path,
    paths: Paths,
    entry: ManifestEntry,
    force: bool,
    codex_model: str,
    codex_effort: str,
) -> str:
    out_path = page_path(paths, entry.page_id)
    if not force and out_path.exists():
        existing = PageSummary.model_validate_json(out_path.read_text())
        if existing.image_sha256 == entry.image_sha256:
            return f"skip {entry.page_id}"

    schema_path = write_temp_schema(PageSummary)
    try:
        prompt = page_prompt(entry)
        tmp_out = out_path.with_suffix(".tmp.json")
        cmd = [
            "codex",
            "exec",
            "--ephemeral",
            "--dangerously-bypass-approvals-and-sandbox",
            "-C",
            root.as_posix(),
            "-m",
            codex_model,
            "-c",
            f'model_reasoning_effort="{codex_effort}"',
            "--image",
            (root / entry.source_image).as_posix(),
            "--output-schema",
            schema_path.as_posix(),
            "-o",
            tmp_out.as_posix(),
            prompt,
        ]
        try:
            subprocess.run(cmd, cwd=root, check=True, text=True, capture_output=True)
        except subprocess.CalledProcessError as exc:
            tmp_out.unlink(missing_ok=True)
            stderr = (exc.stderr or exc.stdout or "").strip()[-4000:]
            raise RuntimeError(f"{entry.page_id} codex failed with exit code {exc.returncode}: {stderr}") from exc
        summary = load_json_model(tmp_out, PageSummary)
        summary = summary.model_copy(
            update={
                "page_id": entry.page_id,
                "volume": entry.volume,
                "page_label": entry.page_label,
                "is_spread": entry.is_spread,
                "source_image": entry.source_image,
                "image_sha256": entry.image_sha256,
                "source_refs": [
                    PageRef(
                        page_id=entry.page_id,
                        source_image=entry.source_image,
                        image_sha256=entry.image_sha256,
                    )
                ],
            }
        )
        write_json_atomic(out_path, summary.model_dump())
        tmp_out.unlink(missing_ok=True)
        if summary.confidence == "low":
            append_jsonl(paths.review_needed, {"page_id": entry.page_id, "reason": "low confidence"})
        return f"wrote {entry.page_id}"
    finally:
        schema_path.unlink(missing_ok=True)


def page_prompt(entry: ManifestEntry) -> str:
    return f"""You are extracting lore from one comic page image for a private research index.

Return only JSON matching the provided schema. Do not write markdown.

Source:
- page_id: {entry.page_id}
- volume: {entry.volume}
- page_label: {entry.page_label}
- source_image: {entry.source_image}
- image_sha256: {entry.image_sha256}

Task:
- Summarize what happens visually and narratively on this page.
- Capture plot events, locations, characters present, dialogue meaning, lore facts, character facts, and uncertainties.
- Do not preserve full verbatim comic text. Short quoted fragments only when indispensable for identification.
- If text is hard to read, mark confidence low or medium and put the issue in uncertainties.
- Keep facts source-grounded; do not infer beyond the page.
"""


def reconcile_volumes(paths: Paths, volume: int, force: bool) -> None:
    pages_by_volume = load_pages_by_volume(paths)
    for vol, pages in sorted(pages_by_volume.items()):
        if volume and vol != volume:
            continue
        out_path = paths.volumes / f"volume-{vol:02d}.json"
        if out_path.exists() and not force:
            print(f"skip volume {vol}")
            continue
        prompt = volume_prompt(vol, pages)
        raw = run_claude_json(prompt, VolumeSummary)
        summary = VolumeSummary.model_validate_json(raw)
        write_json_atomic(out_path, summary.model_dump())
        print(f"wrote {out_path}")


def extract_characters(paths: Paths, volume: int, force: bool) -> None:
    out_path = paths.characters / ("characters.json" if volume == 0 else f"characters-volume-{volume:02d}.json")
    if out_path.exists() and not force:
        print(f"skip {out_path}")
        return
    pages = [
        page
        for page in load_pages(paths)
        if volume == 0 or page.volume == volume
    ]
    volumes = [
        summary
        for summary in load_volumes(paths)
        if volume == 0 or summary.volume == volume
    ]
    prompt = character_prompt(pages, volumes)
    schema = {
        "type": "object",
        "properties": {
            "characters": {
                "type": "array",
                "items": CharacterLore.model_json_schema(),
            }
        },
        "required": ["characters"],
    }
    raw = run_claude_json(prompt, schema)
    data = json.loads(raw)["characters"]
    page_refs = {
        page.page_id: PageRef(
            page_id=page.page_id,
            source_image=page.source_image,
            image_sha256=page.image_sha256,
        )
        for page in pages
    }
    characters = [
        CharacterLore.model_validate(normalize_character_item(item, page_refs)).model_dump()
        for item in data
    ]
    write_json_atomic(out_path, characters)
    print(f"wrote {out_path}")


def normalize_character_item(item: dict[str, Any], page_refs: dict[str, PageRef]) -> dict[str, Any]:
    aliases = list_values(item.get("aliases"))
    name = item.get("name")
    if isinstance(name, str) and name and name not in aliases:
        aliases.insert(0, name)

    character_id = item.get("character_id") or item.get("id")
    if not isinstance(character_id, str) or not character_id:
        character_id = slugify(aliases[0] if aliases else "unknown-character")

    normalized = {
        "character_id": character_id,
        "peer_candidate": bool(item.get("peer_candidate", True)),
        "non_peer_reason": str(item.get("non_peer_reason") or ""),
        "related_peer_ids": string_values(item.get("related_peer_ids")),
        "aliases": aliases,
        "volumes": int_values(item.get("volumes")),
        "appearances": string_values(item.get("appearances")),
        "traits": string_values(item.get("traits")),
        "relationships": string_values(item.get("relationships")),
        "witnessed_facts": string_values(item.get("witnessed_facts") or item.get("facts")),
        "knowledge_boundaries": string_values(item.get("knowledge_boundaries")),
        "important_actions": string_values(item.get("important_actions") or item.get("actions")),
        "voice_notes": string_values(item.get("voice_notes")),
        "source_refs": normalize_source_refs(item, page_refs),
    }
    return normalized


def normalize_source_refs(item: dict[str, Any], page_refs: dict[str, PageRef]) -> list[dict[str, str]]:
    refs: dict[str, PageRef] = {}

    def collect(value: Any) -> None:
        if isinstance(value, dict):
            page_id = value.get("page_id")
            if isinstance(page_id, str) and page_id:
                refs[page_id] = page_refs.get(
                    page_id,
                    PageRef(
                        page_id=page_id,
                        source_image=str(value.get("source_image") or ""),
                        image_sha256=str(value.get("image_sha256") or ""),
                    ),
                )
            for nested in value.values():
                collect(nested)
        elif isinstance(value, list):
            for nested in value:
                collect(nested)

    collect(item.get("source_refs"))
    for key in ("appearances", "relationships", "witnessed_facts", "important_actions", "actions"):
        collect(item.get(key))
    return [ref.model_dump() for _, ref in sorted(refs.items())]


def list_values(value: Any) -> list[str]:
    if value is None:
        return []
    if isinstance(value, str):
        return [value]
    if isinstance(value, list):
        return [str(item) for item in value if item not in (None, "")]
    return [str(value)]


def string_values(value: Any) -> list[str]:
    return [string_value(item) for item in value] if isinstance(value, list) else list_values(value)


def string_value(value: Any) -> str:
    if isinstance(value, str):
        return value
    if isinstance(value, dict):
        parts = [f"{key}: {val}" for key, val in value.items() if val not in (None, "", [])]
        return "; ".join(parts)
    return str(value)


def int_values(value: Any) -> list[int]:
    vals = value if isinstance(value, list) else ([] if value is None else [value])
    out: list[int] = []
    for item in vals:
        try:
            out.append(int(item))
        except (TypeError, ValueError):
            continue
    return out


def slugify(value: str) -> str:
    slug = re.sub(r"[^a-z0-9]+", "-", value.lower()).strip("-")
    return slug or "unknown-character"


def run_claude_json(prompt: str, schema_or_model: Any) -> str:
    schema = json.dumps(schema_for(schema_or_model), ensure_ascii=False)
    cmd = [
        "claude",
        "--model",
        "sonnet",
        "-p",
        "--no-session-persistence",
        "--output-format",
        "json",
        "--effort",
        "medium",
        "--json-schema",
        schema,
    ]
    try:
        result = subprocess.run(
            cmd,
            check=True,
            text=True,
            capture_output=True,
            input=prompt,
            timeout=600,
        )
    except subprocess.CalledProcessError as exc:
        stderr = (exc.stderr or "").strip()
        raise RuntimeError(f"claude failed with exit code {exc.returncode}: {stderr}") from exc
    except subprocess.TimeoutExpired as exc:
        raise RuntimeError("claude timed out after 600 seconds") from exc
    return extract_claude_json(result.stdout)


def extract_claude_json(stdout: str) -> str:
    payload = json.loads(stdout)
    if isinstance(payload, dict):
        if payload.get("is_error"):
            message = payload.get("result") or payload.get("api_error_status") or payload
            raise RuntimeError(f"claude returned an error: {message}")
        structured = payload.get("structured_output")
        if structured is not None:
            return json.dumps(structured, ensure_ascii=False)
        result = payload.get("result")
        if isinstance(result, str) and result.strip():
            return extract_json_text(result)
    return json.dumps(payload, ensure_ascii=False)


def extract_json_text(text: str) -> str:
    stripped = text.strip()
    try:
        json.loads(stripped)
        return stripped
    except json.JSONDecodeError:
        pass

    decoder = json.JSONDecoder()
    for index, char in enumerate(stripped):
        if char not in "{[":
            continue
        try:
            value, _ = decoder.raw_decode(stripped[index:])
        except json.JSONDecodeError:
            continue
        return json.dumps(value, ensure_ascii=False)

    preview = stripped[:1000].replace("\n", "\\n")
    raise RuntimeError(f"claude returned non-JSON result: {preview}")


def volume_prompt(volume: int, pages: list[PageSummary]) -> str:
    payload = [page.model_dump() for page in sorted(pages, key=lambda p: p.page_id)]
    return f"""Reconcile these page-level Metabarons lore summaries into one volume summary.

Rules:
- Use Claude Sonnet.
- Return only JSON matching the provided schema.
- Do not add facts not supported by the page summaries.
- Do not preserve long verbatim comic text.
- Preserve page_ids and source_images for traceability.

Volume: {volume}
Pages JSON:
{json.dumps(payload, ensure_ascii=False)}
"""


def character_prompt(pages: list[PageSummary], volumes: list[VolumeSummary]) -> str:
    payload = {
        "pages": [page.model_dump() for page in sorted(pages, key=lambda p: p.page_id)],
        "volumes": [volume.model_dump() for volume in sorted(volumes, key=lambda v: v.volume)],
    }
    return f"""Extract structured character lore from these Metabarons page and volume summaries.

Rules:
- Use Claude Sonnet.
- Return only JSON shaped like: {{"characters": [CharacterLore, ...]}}.
- Capture aliases, appearances, traits, relationships, witnessed facts, knowledge boundaries, important actions, voice notes, and source refs.
- Source every important character fact with page refs where available.
- Do not invent continuity beyond the source summaries.

Input JSON:
{json.dumps(payload, ensure_ascii=False)}
"""


def ingest_qdrant(paths: Paths, collection: str, dry_run: bool) -> None:
    from .lore_index import build_documents, ingest_index, write_index

    index_dir = paths.root / "lore/packs/metabarons/index"
    docs = build_documents(paths.root, comics_dir=paths.output_dir)
    write_index(paths.root, index_dir, docs, collection=collection)
    ingest_index(index_dir=index_dir, collection=collection, replace=True, dry_run=dry_run)


def smoke_search(root: Path, collection: str) -> None:
    from .lore_index import smoke_index

    smoke_index(index_dir=root / "lore/packs/metabarons/index", collection=collection, strict=False)


def lore_documents(paths: Paths) -> list[dict[str, Any]]:
    from .lore_index import build_documents

    return [doc.qdrant_payload() for doc in build_documents(paths.root, comics_dir=paths.output_dir)]


def validate_outputs(paths: Paths, strict: bool) -> None:
    errors = 0
    for path in sorted(paths.pages.glob("*.json")):
        errors += validate_file(path, PageSummary)
    for path in sorted(paths.volumes.glob("*.json")):
        errors += validate_file(path, VolumeSummary)
    for path in sorted(paths.characters.glob("*.json")):
        data = json.loads(path.read_text())
        items = data if isinstance(data, list) else [data]
        for item in items:
            try:
                CharacterLore.model_validate(item)
            except ValidationError as exc:
                errors += 1
                print(f"{path}: {exc}", file=sys.stderr)
    if errors and strict:
        raise SystemExit(1)
    print(f"validation complete: {errors} errors")


def validate_file(path: Path, model: type[BaseModel]) -> int:
    try:
        model.model_validate_json(path.read_text())
    except ValidationError as exc:
        print(f"{path}: {exc}", file=sys.stderr)
        return 1
    return 0


def load_pages(paths: Paths) -> list[PageSummary]:
    return [PageSummary.model_validate_json(path.read_text()) for path in sorted(paths.pages.glob("*.json"))]


def load_pages_by_volume(paths: Paths) -> dict[int, list[PageSummary]]:
    grouped: dict[int, list[PageSummary]] = defaultdict(list)
    for page in load_pages(paths):
        grouped[page.volume].append(page)
    return grouped


def load_volumes(paths: Paths) -> list[VolumeSummary]:
    return [VolumeSummary.model_validate_json(path.read_text()) for path in sorted(paths.volumes.glob("*.json"))]


def page_path(paths: Paths, page_id: str) -> Path:
    return paths.pages / f"{page_id}.json"


def write_temp_schema(schema_or_model: Any) -> Path:
    schema = schema_for(schema_or_model)
    fd, name = tempfile.mkstemp(prefix="sans-nom-schema-", suffix=".json")
    with os.fdopen(fd, "w") as f:
        json.dump(schema, f)
    return Path(name)


def schema_for(schema_or_model: Any) -> dict[str, Any]:
    if isinstance(schema_or_model, dict):
        schema = schema_or_model
    else:
        schema = schema_or_model.model_json_schema()
    return strict_json_schema(schema)


def strict_json_schema(schema: dict[str, Any]) -> dict[str, Any]:
    """Make Pydantic JSON schema acceptable to strict response-format APIs."""
    copied = json.loads(json.dumps(schema))

    def visit(node: Any) -> None:
        if isinstance(node, dict):
            if node.get("type") == "object" or "properties" in node:
                node["additionalProperties"] = False
                props = node.get("properties")
                if isinstance(props, dict):
                    node["required"] = list(props.keys())
            for value in node.values():
                visit(value)
        elif isinstance(node, list):
            for item in node:
                visit(item)

    visit(copied)
    return copied


def load_json_model(path: Path, model: type[BaseModel]) -> Any:
    return model.model_validate_json(path.read_text())


def write_json_atomic(path: Path, data: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, name = tempfile.mkstemp(prefix=f"{path.name}.", suffix=".tmp", dir=path.parent)
    tmp = Path(name)
    try:
        with os.fdopen(fd, "w") as f:
            json.dump(data, f, ensure_ascii=False, indent=2, sort_keys=True)
            f.write("\n")
        tmp.replace(path)
    finally:
        tmp.unlink(missing_ok=True)


def append_jsonl(path: Path, item: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("a") as f:
        f.write(json.dumps(item, ensure_ascii=False, sort_keys=True) + "\n")


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


if __name__ == "__main__":
    raise SystemExit(main())
