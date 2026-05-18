from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import sys
import tempfile
import uuid
from collections import Counter
from collections.abc import Iterable
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

from pydantic import BaseModel, Field, ValidationError

from .comic_lore import CharacterLore, Paths, load_pages, load_volumes, write_json_atomic
from .env import load_project_env
from .lore_ops import qdrant_url, voyage_api_key, voyage_model

load_project_env()

DEFAULT_PACK_ID = "metabarons"
DEFAULT_PACKS_DIR = Path("lore/packs")
DEFAULT_COLLECTION = "sans_nom_lore_metabarons"
DEFAULT_COMICS_DIR = Path("lore/packs/metabarons/comics")
DEFAULT_INDEX_DIR = Path("lore/packs/metabarons/index")
MAX_MARKDOWN_CHARS = 6000


class IndexDocument(BaseModel):
    doc_id: str
    point_id: str
    kind: str
    source_path: str
    source_sha256: str
    text_sha256: str
    text: str
    volume: int | None = None
    page_id: str | None = None
    character_id: str | None = None
    peer_id: str | None = None
    heading: str | None = None
    payload: dict[str, Any] = Field(default_factory=dict)

    def qdrant_payload(self) -> dict[str, Any]:
        data = self.model_dump(exclude_none=True)
        payload = dict(data.pop("payload", {}))
        payload.update(data)
        return payload


class IndexManifest(BaseModel):
    generated_at: str
    collection: str
    embedding_model: str
    document_count: int
    counts_by_kind: dict[str, int]
    sources: list[dict[str, str]]
    documents: list[dict[str, str]]


class MarkdownChunk(BaseModel):
    doc_id: str
    heading: str
    text: str


class SmokeQuery(BaseModel):
    query: str
    expected_any: list[list[str]]


class LorePack(BaseModel):
    id: str
    label: str
    description: str = ""
    collection: str
    embedding_model: str = "voyage-4"
    artifact_roots: dict[str, str] = Field(default_factory=dict)
    builder: str
    smoke_queries: list[SmokeQuery] = Field(default_factory=list)
    forbidden_literal_terms: list[str] = Field(default_factory=list)
    status: str = "active"


DEFAULT_SMOKE_QUERIES = [
    SmokeQuery(query="Tonto tells a story", expected_any=[["tonto", "story"], ["tonto", "narrat"]]),
    SmokeQuery(query="Metabunker city-shaft", expected_any=[["metabunker"], ["city-shaft"]]),
    SmokeQuery(query="Aghora grief Steelhead", expected_any=[["aghora", "steelhead"]]),
    SmokeQuery(query="Honorata control tenderness", expected_any=[["honorata"]]),
    SmokeQuery(query="Bushitaka law", expected_any=[["bushitaka"]]),
    SmokeQuery(query="Lothar is Steelhead", expected_any=[["lothar", "steelhead"]]),
    SmokeQuery(query="Sans Nom ends the dynasty", expected_any=[["sans-nom", "dynasty"], ["ended", "dynasty"]]),
]


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="lore-index", description="Build and query Sans-Nom lore index.")
    parser.add_argument("--root", default=".", help="Repository root.")
    parser.add_argument("--pack", default=DEFAULT_PACK_ID, help="Ground lore pack id.")
    parser.add_argument("--index-dir", default="", help="Index artifact directory; defaults to the pack index root.")
    parser.add_argument("--collection", default="", help="Qdrant collection name; defaults to the pack collection.")
    sub = parser.add_subparsers(dest="cmd", required=True)

    sub.add_parser("build", help="Build documents.jsonl and manifest.json.")

    validate = sub.add_parser("validate", help="Validate index artifacts.")
    validate.add_argument("--strict", action="store_true")

    sub.add_parser("status", help="Show index artifact status.")

    ingest = sub.add_parser("ingest", help="Embed index documents and upsert to Qdrant.")
    ingest.add_argument("--replace", action="store_true", help="Recreate the Qdrant collection before upsert.")
    ingest.add_argument("--dry-run", action="store_true", help="Validate inputs without contacting Voyage or Qdrant.")
    ingest.add_argument("--fake-embeddings", action="store_true", help="Use deterministic local embeddings.")

    smoke = sub.add_parser("smoke", help="Run retrieval smoke queries against Qdrant.")
    smoke.add_argument("--strict", action="store_true")

    args = parser.parse_args(argv)
    root = Path(args.root).resolve()
    pack = load_lore_pack(root, args.pack)
    index_dir = resolve_index_dir(root, pack, args.index_dir)
    collection = args.collection or pack.collection

    if args.cmd == "build":
        docs = build_documents(root, pack=pack)
        write_index(root, index_dir, docs, collection=collection, embedding_model=pack.embedding_model)
        print(f"wrote {len(docs)} lore index documents to {index_dir}")
        return 0
    if args.cmd == "validate":
        errors = validate_index(root, index_dir)
        print(f"lore-index validation complete: {errors} errors")
        if errors and args.strict:
            return 1
        return 0
    if args.cmd == "status":
        status_index(index_dir)
        return 0
    if args.cmd == "ingest":
        ingest_index(
            index_dir=index_dir,
            collection=collection,
            replace=args.replace,
            dry_run=args.dry_run,
            fake_embeddings=args.fake_embeddings,
            embedding_model=pack.embedding_model,
        )
        return 0
    if args.cmd == "smoke":
        failures = smoke_index(
            index_dir=index_dir,
            collection=collection,
            strict=args.strict,
            smoke_queries=pack.smoke_queries or DEFAULT_SMOKE_QUERIES,
            embedding_model=pack.embedding_model,
        )
        return 1 if failures and args.strict else 0
    raise AssertionError(args.cmd)


def load_lore_pack(root: Path, pack_id: str) -> LorePack:
    pack_id = (pack_id or DEFAULT_PACK_ID).strip()
    path = root / DEFAULT_PACKS_DIR / pack_id / "lore-pack.json"
    try:
        pack = LorePack.model_validate_json(path.read_text())
    except FileNotFoundError as exc:
        raise SystemExit(f"lore pack {pack_id!r} not found at {path}") from exc
    except ValidationError as exc:
        raise SystemExit(f"invalid lore pack {path}: {exc}") from exc
    if pack.id != pack_id:
        raise SystemExit(f"lore pack id mismatch: requested {pack_id!r}, manifest has {pack.id!r}")
    return pack


def pack_root(root: Path, pack: LorePack) -> Path:
    return (root / DEFAULT_PACKS_DIR / pack.id).resolve()


def pack_artifact_path(root: Path, pack: LorePack, key: str, default: str) -> Path:
    value = pack.artifact_roots.get(key) or default
    return (pack_root(root, pack) / value).resolve()


def resolve_index_dir(root: Path, pack: LorePack, override: str = "") -> Path:
    if override:
        return (root / override).resolve()
    return pack_artifact_path(root, pack, "index", "index")


def build_documents(root: Path, comics_dir: Path | None = None, pack: LorePack | None = None) -> list[IndexDocument]:
    root = root.resolve()
    if pack is not None and pack.builder != "metabarons_comics":
        raise ValueError(f"unsupported lore pack builder {pack.builder!r}")
    comics_dir = (
        comics_dir
        or (pack_artifact_path(root, pack, "comics", "comics") if pack else root / DEFAULT_COMICS_DIR)
    ).resolve()
    wiki_path = pack_artifact_path(root, pack, "wiki", "wiki/lore.md") if pack else root / "lore/packs/metabarons/wiki/lore.md"
    links_path = pack_artifact_path(root, pack, "links", "links/matrix.md") if pack else root / "lore/packs/metabarons/links/matrix.md"
    paths = Paths(root=root, output_dir=comics_dir)
    docs: list[IndexDocument] = []

    for page in load_pages(paths):
        source_path = paths.pages / f"{page.page_id}.json"
        docs.append(make_document(
            root=root,
            doc_id=f"comic_page:{page.page_id}",
            kind="comic_page",
            source_path=source_path,
            text=join_text(
                f"Page: {page.page_id}",
                f"Volume: {page.volume}",
                f"Page label: {page.page_label}",
                f"Source image: {page.source_image}",
                f"Characters present: {', '.join(page.characters_present)}",
                f"Locations: {', '.join(page.locations)}",
                section("Visual summary", [page.visual_summary]),
                section("Plot events", page.plot_events),
                section("Dialogue meaning", page.dialogue_meaning),
                section("Lore facts", page.lore_facts),
                section("Character facts", page.character_facts),
                section("Continuity links", page.continuity_links),
                section("Uncertainties", page.uncertainties),
            ),
            volume=page.volume,
            page_id=page.page_id,
            payload={
                "page_label": page.page_label,
                "is_spread": page.is_spread,
                "source_image": page.source_image,
                "image_sha256": page.image_sha256,
                "confidence": page.confidence,
                "characters_present": page.characters_present,
                "locations": page.locations,
            },
        ))

    for volume in load_volumes(paths):
        source_path = paths.volumes / f"volume-{volume.volume:02d}.json"
        docs.append(make_document(
            root=root,
            doc_id=f"comic_volume:volume-{volume.volume:02d}",
            kind="comic_volume",
            source_path=source_path,
            text=join_text(
                f"Volume: {volume.volume}",
                f"Title: {volume.title}",
                section("Chronology", volume.chronology),
                section("Major events", volume.major_events),
                section("Recurring motifs", volume.recurring_motifs),
                section("Lore laws", volume.lore_laws),
                section("Character arcs", flatten_mapping(volume.character_arcs)),
                section("Locations", volume.locations),
                section("Open uncertainties", volume.open_uncertainties),
            ),
            volume=volume.volume,
            payload={"page_ids": volume.page_ids, "source_images": volume.source_images},
        ))

    docs.extend(build_character_documents(root, paths))
    docs.extend(build_peer_card_documents(root, comics_dir / "peer-cards"))
    docs.extend(build_identity_map_documents(root, comics_dir / "peer-cards" / "identity-map.json"))
    docs.extend(build_markdown_documents(root, wiki_path, "wiki_lore"))
    docs.extend(build_markdown_documents(root, links_path, "link_matrix"))

    docs = [doc for doc in docs if doc.text.strip()]
    docs.sort(key=lambda doc: doc.doc_id)
    ensure_unique_documents(docs)
    return docs


def build_character_documents(root: Path, paths: Paths) -> list[IndexDocument]:
    docs: list[IndexDocument] = []
    for path in sorted(paths.characters.glob("*.json")):
        data = json.loads(path.read_text())
        items = data if isinstance(data, list) else [data]
        volume = volume_from_character_path(path)
        for index, item in enumerate(items, start=1):
            char = CharacterLore.model_validate(item)
            record_id = f"{path.name}#{index:03d}:{char.character_id}"
            docs.append(make_document(
                root=root,
                doc_id=f"character_record:{record_id}",
                kind="character_record",
                source_path=path,
                text=join_text(
                    f"Character id: {char.character_id}",
                    f"Aliases: {', '.join(char.aliases)}",
                    section("Appearances", char.appearances),
                    section("Traits", char.traits),
                    section("Relationships", char.relationships),
                    section("Witnessed facts", char.witnessed_facts),
                    section("Knowledge boundaries", char.knowledge_boundaries),
                    section("Important actions", char.important_actions),
                    section("Voice notes", char.voice_notes),
                    f"Non-peer reason: {char.non_peer_reason}" if char.non_peer_reason else "",
                ),
                volume=volume,
                character_id=char.character_id,
                payload={
                    "record_id": record_id,
                    "peer_candidate": char.peer_candidate,
                    "related_peer_ids": char.related_peer_ids,
                    "aliases": char.aliases,
                    "source_refs": [ref.model_dump() for ref in char.source_refs],
                },
            ))
    return docs


def build_peer_card_documents(root: Path, peer_cards_dir: Path) -> list[IndexDocument]:
    docs: list[IndexDocument] = []
    accepted_dir = peer_cards_dir / "cards/accepted"
    non_peer_dir = peer_cards_dir / "cards/non-peer"

    for path in sorted(accepted_dir.glob("*.json")):
        data = json.loads(path.read_text())
        peer_id = data.get("peer_card_id") or path.stem
        card = ensure_list(data.get("card"))
        docs.append(make_document(
            root=root,
            doc_id=f"peer_card:{peer_id}",
            kind="peer_card",
            source_path=path,
            text=join_text(
                f"Peer id: {peer_id}",
                f"Display name: {data.get('display_name', '')}",
                section("Honcho card facts", card),
                f"Phase boundary note: {data.get('phase_boundary_note', '')}",
            ),
            peer_id=peer_id,
            payload={
                "display_name": data.get("display_name"),
                "source_record_ids": ensure_list(data.get("source_record_ids")),
                "source_refs": ensure_list(data.get("source_refs")),
            },
        ))

    for path in sorted(non_peer_dir.glob("*.json")):
        data = json.loads(path.read_text())
        peer_id = data.get("peer_card_id") or path.stem
        report = data.get("verifier_report") if isinstance(data.get("verifier_report"), dict) else {}
        docs.append(make_document(
            root=root,
            doc_id=f"non_peer_lore:{peer_id}",
            kind="non_peer_lore",
            source_path=path,
            text=join_text(
                f"Non-peer id: {peer_id}",
                f"Reason: {data.get('non_peer_reason', '')}",
                section("Manual review notes", ensure_list(report.get("manual_review_notes"))),
                section("Related peer ids", ensure_list(data.get("related_peer_ids"))),
                section("Source record ids", ensure_list(data.get("source_record_ids"))),
            ),
            peer_id=peer_id,
            payload={
                "related_peer_ids": ensure_list(data.get("related_peer_ids")),
                "source_record_ids": ensure_list(data.get("source_record_ids")),
            },
        ))

    return docs


def build_identity_map_documents(root: Path, path: Path) -> list[IndexDocument]:
    if not path.exists():
        return []
    data = json.loads(path.read_text())
    docs: list[IndexDocument] = []
    for peer in ensure_list(data.get("peers")):
        if not isinstance(peer, dict):
            continue
        peer_id = str(peer.get("peer_card_id") or "").strip()
        if not peer_id:
            continue
        docs.append(make_document(
            root=root,
            doc_id=f"identity_map:peer:{peer_id}",
            kind="identity_map_entry",
            source_path=path,
            text=join_text(
                f"Peer id: {peer_id}",
                f"Display name: {peer.get('display_name', '')}",
                f"Arc label: {peer.get('arc_label', '')}",
                f"Merge policy: {peer.get('merge_policy', '')}",
                f"Rationale: {peer.get('rationale', '')}",
                f"Honcho notes: {peer.get('honcho_notes', '')}",
                section("Source record ids", ensure_list(peer.get("source_record_ids"))),
            ),
            peer_id=peer_id,
            payload={"entry_type": "peer", "source_record_ids": ensure_list(peer.get("source_record_ids"))},
        ))

    for item in ensure_list(data.get("non_peer_source_records")):
        if not isinstance(item, dict):
            continue
        source_record_id = str(item.get("source_record_id") or "").strip()
        if not source_record_id:
            continue
        slug = slugify(source_record_id)
        docs.append(make_document(
            root=root,
            doc_id=f"identity_map:non-peer:{slug}",
            kind="identity_map_entry",
            source_path=path,
            text=join_text(
                f"Source record id: {source_record_id}",
                f"Reason: {item.get('reason', '')}",
                section("Related peer ids", ensure_list(item.get("related_peer_ids"))),
            ),
            payload={
                "entry_type": "non_peer",
                "source_record_id": source_record_id,
                "related_peer_ids": ensure_list(item.get("related_peer_ids")),
            },
        ))
    return docs


def build_markdown_documents(root: Path, path: Path, kind: str) -> list[IndexDocument]:
    if not path.exists():
        return []
    docs: list[IndexDocument] = []
    for chunk in split_markdown(path, kind):
        docs.append(make_document(
            root=root,
            doc_id=chunk.doc_id,
            kind=kind,
            source_path=path,
            text=chunk.text,
            heading=chunk.heading,
        ))
    return docs


def make_document(
    *,
    root: Path,
    doc_id: str,
    kind: str,
    source_path: Path,
    text: str,
    volume: int | None = None,
    page_id: str | None = None,
    character_id: str | None = None,
    peer_id: str | None = None,
    heading: str | None = None,
    payload: dict[str, Any] | None = None,
) -> IndexDocument:
    clean_text = normalize_text(text)
    rel_source = relative_path(root, source_path)
    return IndexDocument(
        doc_id=doc_id,
        point_id=point_id_for_doc_id(doc_id),
        kind=kind,
        source_path=rel_source,
        source_sha256=sha256_path(source_path),
        text_sha256=sha256_text(clean_text),
        text=clean_text,
        volume=volume,
        page_id=page_id,
        character_id=character_id,
        peer_id=peer_id,
        heading=heading,
        payload=payload or {},
    )


def write_index(root: Path, index_dir: Path, docs: list[IndexDocument], collection: str, embedding_model: str | None = None) -> None:
    index_dir.mkdir(parents=True, exist_ok=True)
    documents_path = index_dir / "documents.jsonl"
    lines = [
        json.dumps(doc.model_dump(exclude_none=True), ensure_ascii=False, sort_keys=True)
        for doc in docs
    ]
    text = "".join(f"{line}\n" for line in lines)
    write_text_atomic(documents_path, text)
    manifest = build_manifest(docs=docs, collection=collection, embedding_model=embedding_model)
    write_json_atomic(index_dir / "manifest.json", manifest.model_dump())


def build_manifest(docs: list[IndexDocument], collection: str, embedding_model: str | None = None) -> IndexManifest:
    counts = Counter(doc.kind for doc in docs)
    sources: dict[str, str] = {}
    for doc in docs:
        sources[doc.source_path] = doc.source_sha256
    return IndexManifest(
        generated_at=datetime.now(UTC).isoformat(),
        collection=collection,
        embedding_model=embedding_model or voyage_model(),
        document_count=len(docs),
        counts_by_kind=dict(sorted(counts.items())),
        sources=[{"path": path, "sha256": sha} for path, sha in sorted(sources.items())],
        documents=[
            {
                "doc_id": doc.doc_id,
                "kind": doc.kind,
                "source_path": doc.source_path,
                "text_sha256": doc.text_sha256,
            }
            for doc in docs
        ],
    )


def read_index_documents(index_dir: Path) -> list[IndexDocument]:
    path = index_dir / "documents.jsonl"
    docs: list[IndexDocument] = []
    with path.open() as f:
        for line_no, line in enumerate(f, start=1):
            if not line.strip():
                continue
            try:
                docs.append(IndexDocument.model_validate_json(line))
            except ValidationError as exc:
                raise ValueError(f"{path}:{line_no}: {exc}") from exc
    return docs


def validate_index(root: Path, index_dir: Path) -> int:
    errors: list[str] = []
    docs_path = index_dir / "documents.jsonl"
    manifest_path = index_dir / "manifest.json"
    if not docs_path.exists():
        errors.append(f"missing {docs_path}")
    if not manifest_path.exists():
        errors.append(f"missing {manifest_path}")
    if errors:
        for error in errors:
            print(error, file=sys.stderr)
        return len(errors)

    try:
        docs = read_index_documents(index_dir)
    except (OSError, ValueError) as exc:
        print(exc, file=sys.stderr)
        return 1

    try:
        manifest = IndexManifest.model_validate_json(manifest_path.read_text())
    except ValidationError as exc:
        print(f"{manifest_path}: {exc}", file=sys.stderr)
        return 1

    seen_doc_ids: set[str] = set()
    seen_point_ids: set[str] = set()
    for doc in docs:
        if not doc.text.strip():
            errors.append(f"{doc.doc_id}: empty text")
        if doc.doc_id in seen_doc_ids:
            errors.append(f"{doc.doc_id}: duplicate doc_id")
        seen_doc_ids.add(doc.doc_id)
        if doc.point_id in seen_point_ids:
            errors.append(f"{doc.doc_id}: duplicate point_id {doc.point_id}")
        seen_point_ids.add(doc.point_id)
        if doc.point_id != point_id_for_doc_id(doc.doc_id):
            errors.append(f"{doc.doc_id}: unstable point_id")
        if doc.text_sha256 != sha256_text(doc.text):
            errors.append(f"{doc.doc_id}: text_sha256 mismatch")
        source_path = root / doc.source_path
        if not source_path.exists():
            errors.append(f"{doc.doc_id}: missing source_path {doc.source_path}")
        elif doc.source_sha256 != sha256_path(source_path):
            errors.append(f"{doc.doc_id}: source_sha256 mismatch")

    counts = Counter(doc.kind for doc in docs)
    if manifest.document_count != len(docs):
        errors.append("manifest document_count mismatch")
    if manifest.counts_by_kind != dict(sorted(counts.items())):
        errors.append("manifest counts_by_kind mismatch")
    manifest_doc_ids = [item["doc_id"] for item in manifest.documents]
    if manifest_doc_ids != [doc.doc_id for doc in docs]:
        errors.append("manifest document order mismatch")

    for error in errors:
        print(error, file=sys.stderr)
    return len(errors)


def status_index(index_dir: Path) -> None:
    manifest_path = index_dir / "manifest.json"
    if not manifest_path.exists():
        print(f"missing {manifest_path}")
        return
    manifest = IndexManifest.model_validate_json(manifest_path.read_text())
    print(f"collection={manifest.collection}")
    print(f"embedding_model={manifest.embedding_model}")
    print(f"documents={manifest.document_count}")
    for kind, count in sorted(manifest.counts_by_kind.items()):
        print(f"{kind}={count}")


def ingest_index(
    *,
    index_dir: Path,
    collection: str,
    replace: bool,
    dry_run: bool = False,
    fake_embeddings: bool = False,
    embedding_model: str | None = None,
) -> None:
    docs = read_index_documents(index_dir)
    if dry_run:
        print(f"would ingest {len(docs)} docs into {collection}")
        return

    vectors = embed_documents([doc.text for doc in docs], fake=fake_embeddings, embedding_model=embedding_model)
    if not vectors:
        raise RuntimeError("no lore documents to ingest")

    from qdrant_client import QdrantClient
    from qdrant_client.http.models import Distance, PointStruct, VectorParams

    client = QdrantClient(url=qdrant_url(), check_compatibility=False)
    vector_size = len(vectors[0])
    ensure_collection(
        client,
        collection=collection,
        vector_size=vector_size,
        replace=replace,
        vector_params=VectorParams,
        distance=Distance,
    )
    points = [
        PointStruct(id=doc.point_id, vector=vector, payload=doc.qdrant_payload())
        for doc, vector in zip(docs, vectors, strict=True)
    ]
    client.upsert(collection_name=collection, points=points)
    print(f"ingested {len(points)} docs into {collection}")


def smoke_index(
    index_dir: Path,
    collection: str,
    strict: bool,
    smoke_queries: list[SmokeQuery] | None = None,
    embedding_model: str | None = None,
) -> int:
    if not (index_dir / "documents.jsonl").exists():
        raise RuntimeError(f"missing lore index documents at {index_dir / 'documents.jsonl'}")
    key = voyage_api_key()
    if not key:
        raise RuntimeError("SANS_NOM_VOYAGE_API_KEY or VOYAGE_API_KEY is required")

    from voyageai.client import Client as VoyageClient
    from qdrant_client import QdrantClient

    vo = VoyageClient(api_key=key)
    client = QdrantClient(url=qdrant_url(), check_compatibility=False)
    failures = 0
    for smoke in smoke_queries or DEFAULT_SMOKE_QUERIES:
        vector = float_vector(vo.embed([smoke.query], model=embedding_model or voyage_model(), input_type="query").embeddings[0])
        hits = query_points(client, collection, vector, limit=5)
        print(f"\n## {smoke.query}")
        haystacks = []
        for hit in hits:
            payload = hit.payload or {}
            text = " ".join(str(payload.get(key, "")) for key in ("doc_id", "kind", "source_path", "text"))
            haystacks.append(text.lower())
            print(f"- {hit.score:.3f} {payload.get('doc_id')} {payload.get('source_path')}")
        if not smoke_passes(smoke, haystacks):
            failures += 1
            print("MISS: expected relevant top-5 hit", file=sys.stderr)
    if failures:
        print(f"smoke failures: {failures}", file=sys.stderr)
    elif strict:
        print("strict smoke passed")
    return failures


def embed_documents(texts: list[str], fake: bool = False, embedding_model: str | None = None) -> list[list[float]]:
    if fake:
        return [fake_embedding(text) for text in texts]
    key = voyage_api_key()
    if not key:
        raise RuntimeError("SANS_NOM_VOYAGE_API_KEY or VOYAGE_API_KEY is required")
    from voyageai.client import Client as VoyageClient

    vo = VoyageClient(api_key=key)
    batch_size = max(1, int(os.getenv("SANS_NOM_VOYAGE_BATCH_SIZE", "64")))
    vectors: list[list[float]] = []
    for start in range(0, len(texts), batch_size):
        batch = texts[start:start + batch_size]
        response = vo.embed(batch, model=embedding_model or voyage_model(), input_type="document")
        vectors.extend(float_vector(vector) for vector in response.embeddings)
        print(f"embedded {len(vectors)}/{len(texts)} docs", flush=True)
    return vectors


def float_vector(values: Iterable[float | int]) -> list[float]:
    return [float(value) for value in values]


def ensure_collection(
    client: Any,
    *,
    collection: str,
    vector_size: int,
    replace: bool,
    vector_params: Any,
    distance: Any,
) -> None:
    config = vector_params(size=vector_size, distance=distance.COSINE)
    if replace:
        client.recreate_collection(collection_name=collection, vectors_config=config)
        return
    exists = False
    if hasattr(client, "collection_exists"):
        exists = bool(client.collection_exists(collection_name=collection))
    else:
        try:
            client.get_collection(collection_name=collection)
            exists = True
        except Exception:
            exists = False
    if not exists:
        client.create_collection(collection_name=collection, vectors_config=config)


def query_points(
    client: Any,
    collection: str,
    vector: list[float],
    limit: int,
    query_filter: Any | None = None,
) -> list[Any]:
    if hasattr(client, "search"):
        return client.search(
            collection_name=collection,
            query_vector=vector,
            query_filter=query_filter,
            limit=limit,
        )
    return client.query_points(
        collection_name=collection,
        query=vector,
        query_filter=query_filter,
        limit=limit,
    ).points


def smoke_passes(smoke: SmokeQuery, haystacks: list[str]) -> bool:
    for haystack in haystacks:
        for expected in smoke.expected_any:
            if all(term.lower() in haystack for term in expected):
                return True
    return False


def split_markdown(path: Path, kind: str) -> list[MarkdownChunk]:
    lines = path.read_text().splitlines()
    sections: list[tuple[str, list[str]]] = []
    current_heading = path.stem
    current_lines: list[str] = []
    for line in lines:
        match = re.match(r"^(#{1,6})\s+(.+?)\s*$", line)
        if match:
            if current_lines:
                sections.append((current_heading, current_lines))
            current_heading = match.group(2).strip()
            current_lines = [line]
        else:
            current_lines.append(line)
    if current_lines:
        sections.append((current_heading, current_lines))

    chunks: list[MarkdownChunk] = []
    seen_slugs: Counter[str] = Counter()
    for heading, section_lines in sections:
        slug = slugify(heading)
        seen_slugs[slug] += 1
        if seen_slugs[slug] > 1:
            slug = f"{slug}-{seen_slugs[slug]}"
        text = normalize_text("\n".join(section_lines))
        parts = split_long_text(text, MAX_MARKDOWN_CHARS)
        for index, part in enumerate(parts, start=1):
            suffix = "" if len(parts) == 1 else f":part-{index:02d}"
            chunks.append(MarkdownChunk(doc_id=f"{kind}:{slug}{suffix}", heading=heading, text=part))
    return [chunk for chunk in chunks if chunk.text.strip()]


def split_long_text(text: str, max_chars: int) -> list[str]:
    if len(text) <= max_chars:
        return [text]
    paragraphs = re.split(r"\n\s*\n", text)
    chunks: list[str] = []
    current = ""
    for paragraph in paragraphs:
        paragraph = paragraph.strip()
        if not paragraph:
            continue
        if len(paragraph) > max_chars:
            if current:
                chunks.append(current.strip())
                current = ""
            for start in range(0, len(paragraph), max_chars):
                chunks.append(paragraph[start:start + max_chars].strip())
            continue
        next_text = paragraph if not current else f"{current}\n\n{paragraph}"
        if len(next_text) > max_chars:
            chunks.append(current.strip())
            current = paragraph
        else:
            current = next_text
    if current:
        chunks.append(current.strip())
    return chunks


def ensure_unique_documents(docs: Iterable[IndexDocument]) -> None:
    doc_ids: set[str] = set()
    point_ids: set[str] = set()
    for doc in docs:
        if doc.doc_id in doc_ids:
            raise ValueError(f"duplicate doc_id: {doc.doc_id}")
        doc_ids.add(doc.doc_id)
        if doc.point_id in point_ids:
            raise ValueError(f"duplicate point_id: {doc.point_id}")
        point_ids.add(doc.point_id)


def section(label: str, values: Iterable[Any]) -> str:
    lines = [str(value).strip() for value in values if str(value).strip()]
    if not lines:
        return ""
    return "\n".join([f"{label}:"] + [f"- {line}" for line in lines])


def flatten_mapping(mapping: dict[str, list[str]]) -> list[str]:
    lines: list[str] = []
    for key, values in sorted(mapping.items()):
        joined = "; ".join(str(value) for value in values if str(value).strip())
        if joined:
            lines.append(f"{key}: {joined}")
    return lines


def join_text(*parts: str) -> str:
    return normalize_text("\n\n".join(part for part in parts if part and part.strip()))


def normalize_text(text: str) -> str:
    text = text.replace("\r\n", "\n").replace("\r", "\n")
    lines = [line.rstrip() for line in text.splitlines()]
    return "\n".join(lines).strip()


def ensure_list(value: Any) -> list[Any]:
    if value is None:
        return []
    if isinstance(value, list):
        return value
    return [value]


def volume_from_character_path(path: Path) -> int | None:
    match = re.search(r"characters-volume-(\d+)\.json$", path.name)
    if not match:
        return None
    return int(match.group(1))


def point_id_for_doc_id(doc_id: str) -> str:
    return str(uuid.uuid5(uuid.NAMESPACE_URL, f"sans-nom:lore-index:{doc_id}"))


def relative_path(root: Path, path: Path) -> str:
    return path.resolve().relative_to(root.resolve()).as_posix()


def sha256_path(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def sha256_text(text: str) -> str:
    return hashlib.sha256(text.encode("utf-8")).hexdigest()


def slugify(value: str) -> str:
    value = value.lower()
    value = re.sub(r"[^a-z0-9]+", "-", value)
    value = value.strip("-")
    return value or "entry"


def fake_embedding(text: str, dimensions: int = 16) -> list[float]:
    vector = [0.0] * dimensions
    for token in re.findall(r"[a-z0-9]+", text.lower()):
        digest = hashlib.sha256(token.encode("utf-8")).digest()
        index = int.from_bytes(digest[:8], "big") % dimensions
        vector[index] += 1.0
    magnitude = sum(value * value for value in vector) ** 0.5
    if not magnitude:
        return vector
    return [value / magnitude for value in vector]


def write_text_atomic(path: Path, text: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, name = tempfile.mkstemp(prefix=f"{path.name}.", suffix=".tmp", dir=path.parent)
    tmp = Path(name)
    try:
        with os.fdopen(fd, "w") as f:
            f.write(text)
        tmp.replace(path)
    except Exception:
        tmp.unlink(missing_ok=True)
        raise


if __name__ == "__main__":
    raise SystemExit(main())
