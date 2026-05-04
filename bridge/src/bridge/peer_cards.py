from __future__ import annotations

import argparse
import difflib
import json
import os
import re
import signal
import subprocess
import sys
import tempfile
import threading
import time
from collections import Counter
from contextlib import contextmanager
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Iterator, Literal

from pydantic import BaseModel, Field, ValidationError

from .comic_lore import CharacterLore, VolumeSummary, extract_claude_json, schema_for, write_json_atomic
from .env import load_project_env

load_project_env()


def env_int(name: str, default: int) -> int:
    value = os.getenv(name)
    if value is None or not value.strip():
        return default
    try:
        return int(value)
    except ValueError as exc:
        raise ValueError(f"{name} must be an integer, got {value!r}") from exc


DEFAULT_COMICS_DIR = Path("lore/canon/comics")
DEFAULT_PEER_CARDS_DIR = Path("lore/canon/comics/peer-cards")
DEFAULT_IDENTITY_MODEL = "claude-sonnet-4-6"
DEFAULT_IDENTITY_EFFORT = "low"
DEFAULT_CARD_WRITER_BACKEND = "claude"
DEFAULT_CARD_WRITER_MODEL = "claude-sonnet-4-6"
DEFAULT_CARD_WRITER_EFFORT = "max"
DEFAULT_CODEX_OAUTH_WRITER_MODEL = "gpt-5.5"
DEFAULT_CODEX_OAUTH_WRITER_EFFORT = "medium"
DEFAULT_CARD_VERIFIER_MODEL = "gpt-5.5"
DEFAULT_CARD_VERIFIER_EFFORT = "high"
DEFAULT_CARD_VERIFIER_SERVICE_TIER = "fast"
DEFAULT_CARD_WRITER_TIMEOUT = env_int("SANS_NOM_PEER_CARD_WRITER_TIMEOUT", 1800)
DEFAULT_CARD_VERIFIER_TIMEOUT = env_int("SANS_NOM_PEER_CARD_VERIFIER_TIMEOUT", 3600)
DEFAULT_PEER_MATERIAL_TIMEOUT = env_int("SANS_NOM_PEER_MATERIAL_TIMEOUT", 900)
DEFAULT_BATCH_RUNS_DIR = Path(os.getenv("SANS_NOM_PEER_CARD_BATCH_DIR", Path.home() / "Documents/sans-nom-peer-card-runs"))
CODEX_CLI_SERVICE_TIER_ALIASES = {"priority": "fast"}
CODEX_OAUTH_SERVICE_TIER_ALIASES = {"fast": "priority"}
ACTIVE_PROCESS_LOCK = threading.Lock()
ACTIVE_PROCESSES: set[subprocess.Popen[str]] = set()
SIGNAL_HANDLERS_INSTALLED = False
ALLOWED_CARD_PREFIXES = (
    "Name:",
    "ALIAS:",
    "ROLE:",
    "TRAIT:",
    "RELATIONSHIP:",
    "VOICE:",
    "INSTRUCTION:",
    "BOUNDARY:",
)


class SourceCharacterRecord(BaseModel):
    record_id: str
    character_id: str
    source_file: str
    record_index: int
    peer_candidate: bool = True
    non_peer_reason: str = ""
    related_peer_ids: list[str] = Field(default_factory=list)
    aliases: list[str] = Field(default_factory=list)
    volumes: list[int] = Field(default_factory=list)
    page_ids: list[str] = Field(default_factory=list)
    source_images: list[str] = Field(default_factory=list)
    image_sha256s: list[str] = Field(default_factory=list)
    source_refs: list[dict[str, str]] = Field(default_factory=list)
    appearances: list[str] = Field(default_factory=list)
    traits: list[str] = Field(default_factory=list)
    relationships: list[str] = Field(default_factory=list)
    witnessed_facts: list[str] = Field(default_factory=list)
    knowledge_boundaries: list[str] = Field(default_factory=list)
    important_actions: list[str] = Field(default_factory=list)
    voice_notes: list[str] = Field(default_factory=list)


class VolumeContext(BaseModel):
    volume: int
    title: str
    major_events: list[str] = Field(default_factory=list)
    lore_laws: list[str] = Field(default_factory=list)
    open_uncertainties: list[str] = Field(default_factory=list)


MergePolicy = Literal[
    "single_record",
    "merged_same_persona",
    "merged_group_role",
    "split_phase",
    "split_role",
    "split_uncertain",
]


class PeerIdentity(BaseModel):
    peer_card_id: str
    display_name: str
    source_record_ids: list[str]
    arc_label: str
    merge_policy: MergePolicy
    rationale: str
    honcho_notes: str


class NonPeerSourceRecord(BaseModel):
    source_record_id: str
    reason: str
    related_peer_ids: list[str] = Field(default_factory=list)


class IdentityMapDraft(BaseModel):
    peers: list[PeerIdentity]


class IdentityMapModelMetadata(BaseModel):
    writer_model: str
    writer_effort: str
    iterations: int
    generated_at: str


class IdentityMapArtifact(BaseModel):
    version: int
    input_record_count: int
    raw_character_id_count: int
    peer_count: int
    model: IdentityMapModelMetadata
    source_records: list[SourceCharacterRecord]
    peers: list[PeerIdentity]
    non_peer_source_records: list[NonPeerSourceRecord] = Field(default_factory=list)


DETERMINISTIC_PEER_GROUPS: tuple[dict[str, Any], ...] = (
    {
        "peer_card_id": "shabda-oud-mother-superiors",
        "display_name": "Shabda-Oud Mother Superiors",
        "character_ids": frozenset({"mother-superior", "mother-superior-of-the-shabda-oud"}),
        "arc_label": "Shabda-Oud hierarchy leaders / Mother Superior office across appearances",
        "rationale": "Project lore rule: Mother Superiors sit above the Shabda-Oud witches and should stay a separate hierarchy card rather than split by volume.",
        "honcho_notes": "Group-role card for the Mother Superior office; keep separate from ordinary Shabda-Oud witches/priestesses.",
    },
    {
        "peer_card_id": "shabda-oud-witches",
        "display_name": "Shabda-Oud Witches",
        "character_ids": frozenset(
            {
                "the-three-whore-priestesses",
                "the-four-whore-priestesses-of-shabda-oud",
                "the-shabda-oud",
                "shabda_oud",
            }
        ),
        "arc_label": "Recurring Shabda-Oud witches / whore-priestesses as an order-member group across eras",
        "rationale": "Project lore rule: one, two, three, four, or more whore-priestesses/witches are recurring Shabda-Oud order members, not distinct peer personas.",
        "honcho_notes": "Group-role card for ordinary Shabda-Oud witches/priestesses/reverend mothers; encode general traits and group facts, not a single individual.",
    },
)


class ContextEntrypoint(BaseModel):
    path: str
    description: str


class PeerCardInput(BaseModel):
    peer_card_id: str
    identity_map_path: str
    peer_identity: PeerIdentity
    source_records: list[SourceCharacterRecord]
    related_non_peer_source_records: list[dict[str, Any]] = Field(default_factory=list)
    related_peer_ids: list[str] = Field(default_factory=list)
    related_peer_identities: list[PeerIdentity] = Field(default_factory=list)
    context_entrypoints_path: str
    context_entrypoints: dict[str, ContextEntrypoint]
    page_summary_paths: dict[str, str] = Field(default_factory=dict)
    lothar_acceptance_rubric: list[str] = Field(default_factory=list)


class CardDraft(BaseModel):
    peer_card_id: str
    display_name: str
    card: list[str]
    phase_boundary_note: str = ""
    source_strategy: str = ""


class VerifierReview(BaseModel):
    accepted: bool
    blocked_source_insufficient: bool = False
    blocking_issues: list[str] = Field(default_factory=list)
    unsupported_claims: list[str] = Field(default_factory=list)
    format_errors: list[str] = Field(default_factory=list)
    missing_opportunities: list[str] = Field(default_factory=list)
    revision_instructions: list[str] = Field(default_factory=list)
    manual_review_notes: list[str] = Field(default_factory=list)


class PeerMaterialReview(BaseModel):
    peer_card_id: str
    peer_card_material: bool
    should_mark_non_peer_lore: bool = False
    non_peer_reason: str = ""
    related_peer_ids: list[str] = Field(default_factory=list)
    blocking_issues: list[str] = Field(default_factory=list)
    manual_review_notes: list[str] = Field(default_factory=list)


class PeerCardModelMetadata(BaseModel):
    writer_backend: str = DEFAULT_CARD_WRITER_BACKEND
    writer_model: str
    writer_effort: str
    verifier_model: str
    verifier_effort: str
    verifier_service_tier: str = ""
    iterations: int
    generated_at: str


class PeerCardArtifact(BaseModel):
    peer_card_id: str
    display_name: str
    card: list[str]
    phase_boundary_note: str
    source_record_ids: list[str]
    source_refs: list[dict[str, str]]
    verifier_report: VerifierReview
    model: PeerCardModelMetadata


class NonPeerArtifact(BaseModel):
    peer_card_id: str
    source_record_ids: list[str]
    non_peer_reason: str
    related_peer_ids: list[str] = Field(default_factory=list)
    verifier_report: PeerMaterialReview
    generated_at: str


class IdentityValidationError(ValueError):
    def __init__(self, errors: list[str]) -> None:
        super().__init__("\n".join(errors))
        self.errors = errors


def main(argv: list[str] | None = None) -> int:
    install_shutdown_handlers()
    parser = argparse.ArgumentParser(prog="peer-cards", description="Build Honcho peer-card inputs.")
    parser.add_argument("--root", default=".", help="Repository root.")
    parser.add_argument("--comics-dir", default=str(DEFAULT_COMICS_DIR), help="Generated comic lore directory.")
    parser.add_argument("--peer-cards-dir", default=str(DEFAULT_PEER_CARDS_DIR), help="Peer-card output directory.")
    sub = parser.add_subparsers(dest="cmd", required=True)

    identity = sub.add_parser("identity-map", help="Use Sonnet to split/merge extracted character records into peer identities.")
    identity.add_argument("--force", action="store_true", help="Regenerate an existing identity map.")
    identity.add_argument("--max-iterations", type=int, default=5, help="Maximum repair attempts for map coverage/format errors.")
    identity.add_argument("--writer-model", default=os.getenv("SANS_NOM_PEER_IDENTITY_MODEL", DEFAULT_IDENTITY_MODEL))
    identity.add_argument("--writer-effort", default=os.getenv("SANS_NOM_PEER_IDENTITY_EFFORT", DEFAULT_IDENTITY_EFFORT))

    generate = sub.add_parser("generate", help="Generate reviewed Honcho peer cards.")
    generate.add_argument("--peer-id", required=True, help="Peer identity to generate, e.g. lothar.")
    generate.add_argument("--print-card", action="store_true", help="Print only the accepted Honcho card JSON list.")
    generate.add_argument("--force", action="store_true", help="Regenerate even when card.json already exists.")
    generate.add_argument("--max-iterations", type=int, default=0, help="0 means no hard cap.")
    generate.add_argument(
        "--writer-backend",
        choices=("claude", "codex-oauth"),
        default=os.getenv("SANS_NOM_PEER_CARD_WRITER_BACKEND", DEFAULT_CARD_WRITER_BACKEND),
        help="Writer transport. codex-oauth uses the local ~/.codex OAuth bundle through the Go helper.",
    )
    generate.add_argument("--writer-model", default=os.getenv("SANS_NOM_PEER_CARD_WRITER_MODEL", DEFAULT_CARD_WRITER_MODEL))
    generate.add_argument("--writer-effort", default=os.getenv("SANS_NOM_PEER_CARD_WRITER_EFFORT", DEFAULT_CARD_WRITER_EFFORT))
    generate.add_argument(
        "--writer-service-tier",
        default=os.getenv("SANS_NOM_PEER_CARD_WRITER_SERVICE_TIER", os.getenv("SANS_NOM_CODEX_OAUTH_SERVICE_TIER", "")),
        help="Optional direct Codex OAuth service tier. Known unsupported tiers are omitted automatically.",
    )
    generate.add_argument("--verifier-model", default=os.getenv("SANS_NOM_PEER_CARD_VERIFIER_MODEL", DEFAULT_CARD_VERIFIER_MODEL))
    generate.add_argument("--verifier-effort", default=os.getenv("SANS_NOM_PEER_CARD_VERIFIER_EFFORT", DEFAULT_CARD_VERIFIER_EFFORT))
    generate.add_argument("--verifier-service-tier", default=os.getenv("SANS_NOM_PEER_CARD_VERIFIER_SERVICE_TIER", DEFAULT_CARD_VERIFIER_SERVICE_TIER))
    generate.add_argument("--writer-timeout", type=int, default=DEFAULT_CARD_WRITER_TIMEOUT, help="Writer timeout in seconds.")
    generate.add_argument("--verifier-timeout", type=int, default=DEFAULT_CARD_VERIFIER_TIMEOUT, help="Card verifier timeout in seconds.")
    generate.add_argument("--material-timeout", type=int, default=DEFAULT_PEER_MATERIAL_TIMEOUT, help="Peer-material preflight timeout in seconds.")
    generate.add_argument(
        "--skip-material-preflight",
        action="store_true",
        help="Skip the verifier-first peer-material check. Intended only for debugging.",
    )

    batch = sub.add_parser("batch-generate", help="Generate pending peer cards with bounded parallelism.")
    batch.add_argument("--peer-id", action="append", default=[], help="Peer identity to include. Repeatable. Defaults to all pending peers.")
    batch.add_argument("--workers", type=int, default=2)
    batch.add_argument("--limit", type=int, default=0, help="Maximum peers to process after filtering. 0 means no limit.")
    batch.add_argument("--force", action="store_true", help="Regenerate selected peers even when outputs exist.")
    batch.add_argument("--max-iterations", type=int, default=0, help="Passed through to generate. 0 means no hard cap.")
    batch.add_argument(
        "--writer-backend",
        choices=("claude", "codex-oauth"),
        default=os.getenv("SANS_NOM_PEER_CARD_WRITER_BACKEND", DEFAULT_CARD_WRITER_BACKEND),
    )
    batch.add_argument("--writer-model", default=os.getenv("SANS_NOM_PEER_CARD_WRITER_MODEL", DEFAULT_CARD_WRITER_MODEL))
    batch.add_argument("--writer-effort", default=os.getenv("SANS_NOM_PEER_CARD_WRITER_EFFORT", DEFAULT_CARD_WRITER_EFFORT))
    batch.add_argument(
        "--writer-service-tier",
        default=os.getenv("SANS_NOM_PEER_CARD_WRITER_SERVICE_TIER", os.getenv("SANS_NOM_CODEX_OAUTH_SERVICE_TIER", "")),
    )
    batch.add_argument("--verifier-model", default=os.getenv("SANS_NOM_PEER_CARD_VERIFIER_MODEL", DEFAULT_CARD_VERIFIER_MODEL))
    batch.add_argument("--verifier-effort", default=os.getenv("SANS_NOM_PEER_CARD_VERIFIER_EFFORT", DEFAULT_CARD_VERIFIER_EFFORT))
    batch.add_argument("--verifier-service-tier", default=os.getenv("SANS_NOM_PEER_CARD_VERIFIER_SERVICE_TIER", DEFAULT_CARD_VERIFIER_SERVICE_TIER))
    batch.add_argument("--writer-timeout", type=int, default=DEFAULT_CARD_WRITER_TIMEOUT)
    batch.add_argument("--verifier-timeout", type=int, default=DEFAULT_CARD_VERIFIER_TIMEOUT)
    batch.add_argument("--material-timeout", type=int, default=DEFAULT_PEER_MATERIAL_TIMEOUT)
    batch.add_argument("--peer-timeout", type=int, default=0, help="Outer per-peer timeout in seconds. 0 disables the outer timeout.")
    batch.add_argument("--run-dir", default="", help="Directory for batch logs. Defaults under Documents.")
    batch.add_argument("--skip-material-preflight", action="store_true")

    status = sub.add_parser("status", help="Show identity-map status.")
    status.add_argument("--json", action="store_true")

    validate = sub.add_parser("validate", help="Validate peer-card artifacts.")
    validate.add_argument("--strict", action="store_true")

    args = parser.parse_args(argv)
    root = Path(args.root).resolve()
    comics_dir = root / args.comics_dir
    peer_cards_dir = root / args.peer_cards_dir

    if args.cmd == "identity-map":
        out_path = identity_map_path(peer_cards_dir)
        if out_path.exists() and not args.force:
            print(f"skip {out_path}")
            return 0
        artifact = generate_identity_map(
            comics_dir=comics_dir,
            writer_model=args.writer_model,
            writer_effort=args.writer_effort,
            max_iterations=args.max_iterations,
        )
        write_json_atomic(out_path, artifact.model_dump())
        print(f"wrote {out_path} ({artifact.peer_count} peer identities from {artifact.input_record_count} records)")
        return 0

    if args.cmd == "generate":
        artifact = generate_peer_card(
            root=root,
            comics_dir=comics_dir,
            peer_cards_dir=peer_cards_dir,
            peer_id=args.peer_id,
            force=args.force,
            max_iterations=args.max_iterations,
            writer_backend=args.writer_backend,
            writer_model=args.writer_model,
            writer_effort=args.writer_effort,
            writer_service_tier=args.writer_service_tier,
            verifier_model=args.verifier_model,
            verifier_effort=args.verifier_effort,
            verifier_service_tier=args.verifier_service_tier,
            material_preflight=not args.skip_material_preflight,
            writer_timeout=args.writer_timeout,
            verifier_timeout=args.verifier_timeout,
            material_timeout=args.material_timeout,
        )
        if args.print_card:
            if isinstance(artifact, PeerCardArtifact):
                print(json.dumps(artifact.card, ensure_ascii=False, indent=2))
            else:
                print("[]")
        else:
            if isinstance(artifact, PeerCardArtifact):
                print(f"wrote {peer_card_artifact_path(peer_cards_dir, args.peer_id)}")
            else:
                print(f"marked non-peer lore {args.peer_id}: {artifact.non_peer_reason}")
        return 0

    if args.cmd == "batch-generate":
        summary = batch_generate_peer_cards(
            root=root,
            comics_dir=comics_dir,
            peer_cards_dir=peer_cards_dir,
            selected_peer_ids=args.peer_id,
            workers=args.workers,
            limit=args.limit,
            force=args.force,
            max_iterations=args.max_iterations,
            writer_backend=args.writer_backend,
            writer_model=args.writer_model,
            writer_effort=args.writer_effort,
            writer_service_tier=args.writer_service_tier,
            verifier_model=args.verifier_model,
            verifier_effort=args.verifier_effort,
            verifier_service_tier=args.verifier_service_tier,
            material_preflight=not args.skip_material_preflight,
            writer_timeout=args.writer_timeout,
            verifier_timeout=args.verifier_timeout,
            material_timeout=args.material_timeout,
            peer_timeout=args.peer_timeout,
            run_dir=Path(args.run_dir) if args.run_dir else None,
        )
        print(
            f"batch summary accepted={summary['accepted']} non_peer={summary['non_peer']} "
            f"failed={summary['failed']} skipped={summary['skipped']} run_dir={summary['run_dir']}"
        )
        return 1 if summary["failed"] else 0

    if args.cmd == "status":
        summary = identity_status(peer_cards_dir)
        if args.json:
            print(json.dumps(summary, ensure_ascii=False, indent=2, sort_keys=True))
        else:
            print(
                f"identity_map={summary['identity_map']} "
                f"records={summary['input_record_count']} "
                f"raw_ids={summary['raw_character_id_count']} "
                f"peers={summary['peer_count']}"
            )
        return 0

    if args.cmd == "validate":
        errors = validate_peer_card_outputs(peer_cards_dir)
        print(f"peer-card validation complete: {len(errors)} errors")
        if errors:
            for error in errors:
                print(error)
            if args.strict:
                return 1
        return 0

    raise AssertionError(args.cmd)


def identity_map_path(peer_cards_dir: Path) -> Path:
    return peer_cards_dir / "identity-map.json"


def peer_card_artifact_path(peer_cards_dir: Path, peer_id: str) -> Path:
    return peer_cards_dir / "cards" / "accepted" / f"{peer_id}.json"


def non_peer_artifact_path(peer_cards_dir: Path, peer_id: str) -> Path:
    return peer_cards_dir / "cards" / "non-peer" / f"{peer_id}.json"


def peer_input_path(peer_cards_dir: Path, peer_id: str) -> Path:
    return peer_cards_dir / "provenance" / "inputs" / f"{peer_id}.input.json"


def peer_material_review_path(peer_cards_dir: Path, peer_id: str) -> Path:
    return peer_cards_dir / "provenance" / "material-reviews" / f"{peer_id}.review.json"


def peer_iterations_dir(peer_cards_dir: Path, peer_id: str) -> Path:
    return peer_cards_dir / "runs" / "iterations" / peer_id


def context_entrypoints_path(peer_cards_dir: Path) -> Path:
    return peer_cards_dir / "context-entrypoints.json"


def generate_identity_map(
    comics_dir: Path,
    writer_model: str,
    writer_effort: str,
    max_iterations: int,
) -> IdentityMapArtifact:
    source_records = load_source_character_records(comics_dir)
    volumes = load_volume_context(comics_dir)
    non_peer_records = non_peer_source_records_for(source_records)
    peer_source_records = [record for record in source_records if record.peer_candidate]
    singleton_peers, duplicate_records = split_singletons_and_duplicates(peer_source_records)
    previous_output: str | None = None
    previous_errors: list[str] = []

    for iteration in range(1, max(1, max_iterations) + 1):
        with tempfile_json(identity_input_payload(duplicate_records, volumes)) as input_path:
            prompt = identity_map_prompt(
                input_path=input_path,
                previous_output=previous_output,
                previous_errors=previous_errors,
            )
            raw = run_claude_json(
                prompt=prompt,
                schema_or_model=IdentityMapDraft,
                model=writer_model,
                effort=writer_effort,
                timeout=900,
            )
        previous_output = raw
        try:
            draft = IdentityMapDraft.model_validate_json(raw)
            draft.peers = singleton_peers + draft.peers
            draft = apply_deterministic_peer_overrides(draft, source_records)
            normalize_peer_ids(draft)
            validate_identity_draft(draft, source_records, non_peer_records)
        except (ValidationError, IdentityValidationError) as exc:
            previous_errors = exc.errors if isinstance(exc, IdentityValidationError) else [str(exc)]
            continue

        return IdentityMapArtifact(
            version=1,
            input_record_count=len(source_records),
            raw_character_id_count=len({record.character_id for record in source_records}),
            peer_count=len(draft.peers),
            model=IdentityMapModelMetadata(
                writer_model=writer_model,
                writer_effort=writer_effort,
                iterations=iteration,
                generated_at=datetime.now(timezone.utc).isoformat(),
            ),
            source_records=source_records,
            peers=draft.peers,
            non_peer_source_records=non_peer_records,
        )

    raise RuntimeError(f"identity-map generation failed after {max_iterations} iterations: {previous_errors}")


def non_peer_source_records_for(source_records: list[SourceCharacterRecord]) -> list[NonPeerSourceRecord]:
    return [
        NonPeerSourceRecord(
            source_record_id=record.record_id,
            reason=record.non_peer_reason or "Source record is lore evidence, not a single Honcho peer character.",
            related_peer_ids=record.related_peer_ids,
        )
        for record in source_records
        if not record.peer_candidate
    ]


def apply_deterministic_peer_overrides(
    draft: IdentityMapDraft,
    source_records: list[SourceCharacterRecord],
) -> IdentityMapDraft:
    group_record_ids: dict[str, list[str]] = {}
    group_for_record_id: dict[str, str] = {}
    for group in DETERMINISTIC_PEER_GROUPS:
        character_ids = group["character_ids"]
        record_ids = [record.record_id for record in source_records if record.character_id in character_ids]
        if not record_ids:
            continue
        peer_card_id = str(group["peer_card_id"])
        group_record_ids[peer_card_id] = record_ids
        for record_id in record_ids:
            group_for_record_id[record_id] = peer_card_id

    if not group_for_record_id:
        return draft

    group_peers = {
        str(group["peer_card_id"]): PeerIdentity(
            peer_card_id=str(group["peer_card_id"]),
            display_name=str(group["display_name"]),
            source_record_ids=group_record_ids[str(group["peer_card_id"])],
            arc_label=str(group["arc_label"]),
            merge_policy="merged_group_role",
            rationale=str(group["rationale"]),
            honcho_notes=str(group["honcho_notes"]),
        )
        for group in DETERMINISTIC_PEER_GROUPS
        if str(group["peer_card_id"]) in group_record_ids
    }

    peers: list[PeerIdentity] = []
    inserted_group_ids: set[str] = set()
    for peer in draft.peers:
        peer_group_ids: list[str] = []
        remaining_record_ids: list[str] = []
        for record_id in peer.source_record_ids:
            group_id = group_for_record_id.get(record_id)
            if group_id is None:
                remaining_record_ids.append(record_id)
            elif group_id not in peer_group_ids:
                peer_group_ids.append(group_id)
        for group_id in peer_group_ids:
            if group_id not in inserted_group_ids:
                peers.append(group_peers[group_id])
                inserted_group_ids.add(group_id)
        if remaining_record_ids:
            peers.append(peer.model_copy(update={"source_record_ids": remaining_record_ids}))

    for group_id, group_peer in group_peers.items():
        if group_id not in inserted_group_ids:
            peers.append(group_peer)

    return IdentityMapDraft(peers=peers)


def split_singletons_and_duplicates(
    source_records: list[SourceCharacterRecord],
) -> tuple[list[PeerIdentity], list[SourceCharacterRecord]]:
    duplicate_record_ids = alias_linked_duplicate_record_ids(source_records)
    singletons: list[PeerIdentity] = []
    duplicates: list[SourceCharacterRecord] = []
    for record in source_records:
        if record.record_id in duplicate_record_ids:
            duplicates.append(record)
            continue
        display_name = record.aliases[0] if record.aliases else record.character_id.replace("-", " ").title()
        singletons.append(
            PeerIdentity(
                peer_card_id=slugify_peer_id(record.character_id),
                display_name=display_name,
                source_record_ids=[record.record_id],
                arc_label="single extracted record",
                merge_policy="single_record",
                rationale="Only one extracted record uses this character_id, so no phase split is needed in the identity-map pass.",
                honcho_notes="Deterministic singleton peer; review during peer-card generation if the source proves too thin.",
            )
        )
    return singletons, duplicates


def alias_linked_duplicate_record_ids(source_records: list[SourceCharacterRecord]) -> set[str]:
    key_to_records: dict[str, set[str]] = {}
    for record in source_records:
        for key in identity_keys(record):
            key_to_records.setdefault(key, set()).add(record.record_id)
    duplicate_ids: set[str] = set()
    for record_ids in key_to_records.values():
        if len(record_ids) > 1:
            duplicate_ids.update(record_ids)
    keys = sorted(key_to_records)
    for index, left in enumerate(keys):
        for right in keys[index + 1 :]:
            if not likely_ocr_variant_key(left, right):
                continue
            duplicate_ids.update(key_to_records[left])
            duplicate_ids.update(key_to_records[right])
    return duplicate_ids


def identity_keys(record: SourceCharacterRecord) -> set[str]:
    values = {record.character_id.replace("_", " ").replace("-", " "), *record.aliases}
    return {key for value in values if (key := normalize_identity_key(value))}


GENERIC_IDENTITY_KEYS = {
    "baron",
    "emperor",
    "empress",
    "master",
    "metabaron",
    "new metabaron",
    "current metabaron",
    "supreme metabaron",
    "member of imperial couple",
}


def normalize_identity_key(value: str) -> str:
    key = value.lower().strip()
    key = re.sub(r"^(the|a|an)\s+", "", key)
    key = re.sub(r"[^a-z0-9]+", " ", key)
    key = re.sub(r"\s+", " ", key).strip()
    if len(key) < 4 or key in GENERIC_IDENTITY_KEYS:
        return ""
    return key


def likely_ocr_variant_key(left: str, right: str) -> bool:
    if left == right or abs(len(left) - len(right)) > 3:
        return False
    left_tokens = left.split()
    right_tokens = right.split()
    if len(left_tokens) != len(right_tokens):
        return False
    if left_tokens[0][0] != right_tokens[0][0]:
        return False
    ratio = difflib.SequenceMatcher(None, left, right).ratio()
    compact_max_len = max(len(left.replace(" ", "")), len(right.replace(" ", "")))
    if ratio < 0.86:
        if compact_max_len > 8 or ratio < 0.70:
            return False
    changed_tokens = sum(1 for left_token, right_token in zip(left_tokens, right_tokens) if left_token != right_token)
    return changed_tokens == 1 or (compact_max_len <= 8 and changed_tokens == 2)


def identity_input_payload(
    duplicate_records: list[SourceCharacterRecord],
    volumes: list[VolumeContext],
) -> dict[str, Any]:
    duplicate_ids = sorted({record.character_id for record in duplicate_records})
    relevant_volumes = {
        volume
        for record in duplicate_records
        for volume in record.volumes
    }
    return {
        "task": "Resolve duplicate or alias-linked character records into Honcho peer-card identities. Singleton records are handled deterministically outside this model call.",
        "duplicate_character_ids": duplicate_ids,
        "duplicate_source_records": [compact_source_record(record) for record in duplicate_records],
        "volume_context": [
            volume.model_dump()
            for volume in volumes
            if volume.volume in relevant_volumes
        ],
    }


def load_source_character_records(comics_dir: Path) -> list[SourceCharacterRecord]:
    records: list[SourceCharacterRecord] = []
    for source_path in sorted((comics_dir / "characters").glob("characters-volume-*.json")):
        data = json.loads(source_path.read_text())
        for index, item in enumerate(data):
            character = CharacterLore.model_validate(item)
            page_ids = sorted({ref.page_id for ref in character.source_refs})
            source_images = sorted({ref.source_image for ref in character.source_refs})
            image_sha256s = sorted({ref.image_sha256 for ref in character.source_refs})
            source_refs = [
                {
                    "page_id": ref.page_id,
                    "source_image": ref.source_image,
                    "image_sha256": ref.image_sha256,
                }
                for ref in character.source_refs
            ]
            records.append(
                SourceCharacterRecord(
                    record_id=f"{source_path.name}#{index:03d}:{character.character_id}",
                    character_id=character.character_id,
                    source_file=source_path.as_posix(),
                    record_index=index,
                    peer_candidate=character.peer_candidate,
                    non_peer_reason=character.non_peer_reason,
                    related_peer_ids=dedupe(character.related_peer_ids),
                    aliases=dedupe(character.aliases),
                    volumes=sorted(set(character.volumes)),
                    page_ids=page_ids,
                    source_images=source_images,
                    image_sha256s=image_sha256s,
                    source_refs=source_refs,
                    appearances=dedupe(character.appearances),
                    traits=dedupe(character.traits),
                    relationships=dedupe(character.relationships),
                    witnessed_facts=dedupe(character.witnessed_facts),
                    knowledge_boundaries=dedupe(character.knowledge_boundaries),
                    important_actions=dedupe(character.important_actions),
                    voice_notes=dedupe(character.voice_notes),
                )
            )
    return records


def load_volume_context(comics_dir: Path) -> list[VolumeContext]:
    contexts: list[VolumeContext] = []
    for path in sorted((comics_dir / "volumes").glob("volume-*.json")):
        volume = VolumeSummary.model_validate_json(path.read_text())
        contexts.append(
            VolumeContext(
                volume=volume.volume,
                title=volume.title,
                major_events=volume.major_events,
                lore_laws=volume.lore_laws,
                open_uncertainties=volume.open_uncertainties,
            )
        )
    return contexts


def generate_peer_card(
    root: Path,
    comics_dir: Path,
    peer_cards_dir: Path,
    peer_id: str,
    force: bool,
    max_iterations: int,
    writer_model: str,
    writer_effort: str,
    verifier_model: str,
    verifier_effort: str,
    verifier_service_tier: str = DEFAULT_CARD_VERIFIER_SERVICE_TIER,
    writer_backend: str = DEFAULT_CARD_WRITER_BACKEND,
    writer_service_tier: str = "",
    material_preflight: bool = False,
    writer_timeout: int = DEFAULT_CARD_WRITER_TIMEOUT,
    verifier_timeout: int = DEFAULT_CARD_VERIFIER_TIMEOUT,
    material_timeout: int = DEFAULT_PEER_MATERIAL_TIMEOUT,
) -> PeerCardArtifact | NonPeerArtifact:
    out_path = peer_card_artifact_path(peer_cards_dir, peer_id)
    if out_path.exists() and not force:
        return PeerCardArtifact.model_validate_json(out_path.read_text())

    non_peer_path = non_peer_artifact_path(peer_cards_dir, peer_id)
    if non_peer_path.exists() and not force:
        return NonPeerArtifact.model_validate_json(non_peer_path.read_text())

    iterations_dir = peer_iterations_dir(peer_cards_dir, peer_id)
    iterations_dir.mkdir(parents=True, exist_ok=True)
    peer_input = build_peer_input(root, comics_dir, peer_cards_dir, peer_id)
    input_path = peer_input_path(peer_cards_dir, peer_id)
    write_json_atomic(input_path, peer_input.model_dump())

    if material_preflight:
        material_review = run_peer_material_verifier(
            root=root,
            input_path=input_path,
            model=verifier_model,
            effort=verifier_effort,
            service_tier=verifier_service_tier,
            timeout=material_timeout,
        )
        if (
            material_review.peer_card_id == peer_id
            and not material_review.peer_card_material
            and material_review.should_mark_non_peer_lore
        ):
            artifact = mark_peer_as_non_peer_lore(
                comics_dir=comics_dir,
                peer_cards_dir=peer_cards_dir,
                peer_input=peer_input,
                review=material_review,
            )
            write_json_atomic(non_peer_path, artifact.model_dump())
            return artifact
        preflight_path = peer_material_review_path(peer_cards_dir, peer_id)
        write_json_atomic(preflight_path, material_review.model_dump())

    previous_review_path: Path | None = None
    iteration = 1
    if not force:
        latest = latest_review(iterations_dir)
        if latest is not None:
            last_iteration, previous_review_path, previous_review = latest
            if previous_review.accepted:
                draft_path = iterations_dir / f"{last_iteration:02d}-draft.json"
                draft = CardDraft.model_validate_json(draft_path.read_text())
                artifact = build_peer_card_artifact(
                    peer_input=peer_input,
                    draft=draft,
                    review=previous_review,
                    writer_backend=writer_backend,
                    writer_model=writer_model,
                    writer_effort=writer_effort,
                    verifier_model=verifier_model,
                    verifier_effort=verifier_effort,
                    verifier_service_tier=verifier_service_tier,
                    iteration=last_iteration,
                )
                write_json_atomic(out_path, artifact.model_dump())
                return artifact
            if previous_review.blocked_source_insufficient:
                raise RuntimeError(f"{peer_id} blocked as source-insufficient")
            iteration = last_iteration + 1
        latest_iteration = latest_iteration_number(iterations_dir)
        if latest_iteration is not None:
            iteration = max(iteration, latest_iteration + 1)

    while True:
        if max_iterations and iteration > max_iterations:
            raise RuntimeError(f"{peer_id} failed to reach accepted review after {max_iterations} iterations")
        draft_path = iterations_dir / f"{iteration:02d}-draft.json"
        review_path = iterations_dir / f"{iteration:02d}-review.json"

        draft = run_writer(
            root=root,
            input_path=input_path,
            previous_review_path=previous_review_path,
            backend=writer_backend,
            model=writer_model,
            effort=writer_effort,
            service_tier=writer_service_tier,
            timeout=writer_timeout,
        )
        write_json_atomic(draft_path, draft.model_dump())

        local_errors = validate_card_contract(draft.card)
        if peer_id == "lothar":
            local_errors.extend(validate_lothar_card_rubric(draft.card))
        review = run_verifier(
            root=root,
            input_path=input_path,
            draft_path=draft_path,
            model=verifier_model,
            effort=verifier_effort,
            service_tier=verifier_service_tier,
            local_errors=local_errors,
            timeout=verifier_timeout,
        )
        if local_errors and review.accepted:
            review.accepted = False
            review.format_errors.extend(local_errors)
            review.revision_instructions.append("Fix the deterministic local validation errors before approval.")
        write_json_atomic(review_path, review.model_dump())

        if review.accepted:
            artifact = build_peer_card_artifact(
                peer_input=peer_input,
                draft=draft,
                review=review,
                writer_backend=writer_backend,
                writer_model=writer_model,
                writer_effort=writer_effort,
                verifier_model=verifier_model,
                verifier_effort=verifier_effort,
                verifier_service_tier=verifier_service_tier,
                iteration=iteration,
            )
            write_json_atomic(out_path, artifact.model_dump())
            return artifact
        if review.blocked_source_insufficient:
            write_json_atomic(peer_cards_dir / "runs" / "blocked" / f"{peer_id}.json", review.model_dump())
            raise RuntimeError(f"{peer_id} blocked as source-insufficient")

        previous_review_path = review_path
        iteration += 1


def batch_generate_peer_cards(
    root: Path,
    comics_dir: Path,
    peer_cards_dir: Path,
    selected_peer_ids: list[str],
    workers: int,
    limit: int,
    force: bool,
    max_iterations: int,
    writer_backend: str,
    writer_model: str,
    writer_effort: str,
    writer_service_tier: str,
    verifier_model: str,
    verifier_effort: str,
    verifier_service_tier: str,
    material_preflight: bool,
    writer_timeout: int,
    verifier_timeout: int,
    material_timeout: int,
    peer_timeout: int,
    run_dir: Path | None,
) -> dict[str, Any]:
    if workers < 1:
        raise ValueError("--workers must be at least 1")
    peers = batch_peer_ids(peer_cards_dir, selected_peer_ids, force=force)
    if limit:
        peers = peers[:limit]
    now = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    run_dir = run_dir or DEFAULT_BATCH_RUNS_DIR / f"batch-generate-{now}-w{workers}"
    run_dir.mkdir(parents=True, exist_ok=True)
    batch_log = run_dir / "batch.jsonl"
    write_batch_event(
        batch_log,
        {
            "event": "batch_start",
            "ts": datetime.now(timezone.utc).isoformat(),
            "pending": len(peers),
            "workers": workers,
            "writer_backend": writer_backend,
            "writer_model": writer_model,
            "writer_effort": writer_effort,
        },
    )
    print(f"BATCH_START pending={len(peers)} workers={workers} run_dir={run_dir}", flush=True)

    active: dict[subprocess.Popen[str], dict[str, Any]] = {}
    active_logs: dict[subprocess.Popen[str], Any] = {}
    active_peer_ids: set[str] = set()
    index = 0
    stats = Counter()

    try:
        while index < len(peers) or active:
            while index < len(peers) and len(active) < workers:
                peer_id = peers[index]
                index += 1
                card_path = peer_card_artifact_path(peer_cards_dir, peer_id)
                non_peer_path = non_peer_artifact_path(peer_cards_dir, peer_id)
                if not force and card_path.exists():
                    stats["skipped"] += 1
                    print(f"SKIP {index}/{len(peers)} {peer_id} already-card", flush=True)
                    continue
                if not force and non_peer_path.exists():
                    stats["skipped"] += 1
                    print(f"SKIP {index}/{len(peers)} {peer_id} already-non-peer", flush=True)
                    continue
                log_path = run_dir / f"{peer_id}.log"
                log_file = log_path.open("w")
                cmd = batch_generate_command(
                    root=root,
                    comics_dir=comics_dir,
                    peer_cards_dir=peer_cards_dir,
                    peer_id=peer_id,
                    force=force,
                    max_iterations=max_iterations,
                    writer_backend=writer_backend,
                    writer_model=writer_model,
                    writer_effort=writer_effort,
                    writer_service_tier=writer_service_tier,
                    verifier_model=verifier_model,
                    verifier_effort=verifier_effort,
                    verifier_service_tier=verifier_service_tier,
                    material_preflight=material_preflight,
                    writer_timeout=writer_timeout,
                    verifier_timeout=verifier_timeout,
                    material_timeout=material_timeout,
                )
                log_file.write("$ " + " ".join(cmd) + "\n")
                log_file.flush()
                process = subprocess.Popen(
                    cmd,
                    cwd=root,
                    text=True,
                    stdout=log_file,
                    stderr=subprocess.STDOUT,
                    start_new_session=True,
                )
                active[process] = {
                    "peer_id": peer_id,
                    "index": index,
                    "started": time.time(),
                    "log_path": log_path,
                    "timed_out": False,
                }
                active_logs[process] = log_file
                active_peer_ids.add(peer_id)
                print(f"START {index}/{len(peers)} {peer_id}", flush=True)

            time.sleep(1)
            for process, meta in list(active.items()):
                if peer_timeout and time.time() - meta["started"] > peer_timeout and process.poll() is None:
                    meta["timed_out"] = True
                    terminate_process_group(process)
                if process.poll() is None:
                    continue
                active_logs.pop(process).close()
                active.pop(process)
                active_peer_ids.discard(meta["peer_id"])
                status = batch_peer_status(peer_cards_dir, meta["peer_id"])
                if status == "missing":
                    status = "failed"
                stats[status] += 1
                elapsed = round(time.time() - meta["started"], 2)
                row = {
                    "index": meta["index"],
                    "total": len(peers),
                    "peer_id": meta["peer_id"],
                    "status": status,
                    "returncode": process.returncode,
                    "elapsed_seconds": elapsed,
                    "finished_at": datetime.now(timezone.utc).isoformat(),
                    "log_path": str(meta["log_path"]),
                    "timed_out": meta["timed_out"],
                }
                write_batch_event(batch_log, row)
                print(f"DONE  {meta['index']}/{len(peers)} {meta['peer_id']} status={status} rc={process.returncode} elapsed={elapsed}s", flush=True)
                if status == "failed":
                    print(f"FAILTAIL {meta['peer_id']} {tail_for_log(meta['log_path'])}", flush=True)
    except BaseException:
        for process in list(active):
            terminate_process_group(process)
        sweep_orphan_peer_processes(peer_cards_dir, active_peer_ids)
        raise
    finally:
        for log_file in list(active_logs.values()):
            try:
                log_file.close()
            except OSError:
                pass
        sweep_orphan_peer_processes(peer_cards_dir, active_peer_ids)

    return {
        "accepted": stats["accepted"],
        "non_peer": stats["non-peer"],
        "failed": stats["failed"],
        "skipped": stats["skipped"],
        "run_dir": str(run_dir),
    }


def batch_peer_ids(peer_cards_dir: Path, selected_peer_ids: list[str], force: bool) -> list[str]:
    artifact = IdentityMapArtifact.model_validate_json(identity_map_path(peer_cards_dir).read_text())
    selected = set(selected_peer_ids)
    valid = {peer.peer_card_id for peer in artifact.peers}
    unknown = selected - valid
    if unknown:
        raise ValueError(f"unknown peer ids: {', '.join(sorted(unknown))}")
    peers: list[str] = []
    for peer in artifact.peers:
        if selected and peer.peer_card_id not in selected:
            continue
        if not force and peer_card_artifact_path(peer_cards_dir, peer.peer_card_id).exists():
            continue
        if not force and non_peer_artifact_path(peer_cards_dir, peer.peer_card_id).exists():
            continue
        peers.append(peer.peer_card_id)
    return peers


def batch_generate_command(
    root: Path,
    comics_dir: Path,
    peer_cards_dir: Path,
    peer_id: str,
    force: bool,
    max_iterations: int,
    writer_backend: str,
    writer_model: str,
    writer_effort: str,
    writer_service_tier: str,
    verifier_model: str,
    verifier_effort: str,
    verifier_service_tier: str,
    material_preflight: bool,
    writer_timeout: int,
    verifier_timeout: int,
    material_timeout: int,
) -> list[str]:
    cmd = [
        sys.executable,
        "-m",
        "bridge.peer_cards",
        "--root",
        root.as_posix(),
        "--comics-dir",
        path_arg(comics_dir, root),
        "--peer-cards-dir",
        path_arg(peer_cards_dir, root),
        "generate",
        "--peer-id",
        peer_id,
        "--max-iterations",
        str(max_iterations),
        "--writer-backend",
        writer_backend,
        "--writer-model",
        writer_model,
        "--writer-effort",
        writer_effort,
        "--verifier-model",
        verifier_model,
        "--verifier-effort",
        verifier_effort,
        "--verifier-service-tier",
        verifier_service_tier,
        "--writer-timeout",
        str(writer_timeout),
        "--verifier-timeout",
        str(verifier_timeout),
        "--material-timeout",
        str(material_timeout),
    ]
    if writer_service_tier:
        cmd.extend(["--writer-service-tier", writer_service_tier])
    if force:
        cmd.append("--force")
    if not material_preflight:
        cmd.append("--skip-material-preflight")
    return cmd


def path_arg(path: Path, root: Path) -> str:
    try:
        return path.relative_to(root).as_posix()
    except ValueError:
        return path.as_posix()


def batch_peer_status(peer_cards_dir: Path, peer_id: str) -> str:
    if peer_card_artifact_path(peer_cards_dir, peer_id).exists():
        return "accepted"
    if non_peer_artifact_path(peer_cards_dir, peer_id).exists():
        return "non-peer"
    return "missing"


def write_batch_event(batch_log: Path, row: dict[str, Any]) -> None:
    with batch_log.open("a") as f:
        f.write(json.dumps(row, sort_keys=True) + "\n")


def tail_for_log(log_path: Path, limit: int = 1200) -> str:
    try:
        return log_path.read_text(errors="replace")[-limit:].replace("\n", "\\n")
    except OSError as exc:
        return f"<failed to read log: {exc}>"


def sweep_orphan_peer_processes(peer_cards_dir: Path, peer_ids: set[str]) -> None:
    if not peer_ids or os.name == "nt":
        return
    try:
        result = subprocess.run(
            ["ps", "-axo", "pid=,pgid=,command="],
            text=True,
            capture_output=True,
            check=False,
        )
    except OSError:
        return
    patterns = [pattern for peer_id in peer_ids for pattern in (
        peer_input_path(peer_cards_dir, peer_id).as_posix(),
        f"{peer_iterations_dir(peer_cards_dir, peer_id).as_posix()}/",
    )]
    for line in result.stdout.splitlines():
        parts = line.strip().split(None, 2)
        if len(parts) != 3:
            continue
        pid_text, pgid_text, command = parts
        if not any(pattern in command for pattern in patterns):
            continue
        if not any(marker in command for marker in ("claude ", "codex exec", "codex-responses-json")):
            continue
        try:
            pgid = int(pgid_text)
            if pgid == os.getpgrp():
                continue
            os.killpg(pgid, signal.SIGTERM)
        except (ValueError, ProcessLookupError, PermissionError):
            continue


def latest_review(iterations_dir: Path) -> tuple[int, Path, VerifierReview] | None:
    latest: tuple[int, Path, VerifierReview] | None = None
    for review_path in sorted(iterations_dir.glob("*-review.json")):
        try:
            iteration = int(review_path.name.split("-", 1)[0])
            review = VerifierReview.model_validate_json(review_path.read_text())
        except (ValueError, ValidationError, OSError):
            continue
        latest = (iteration, review_path, review)
    return latest


def latest_iteration_number(iterations_dir: Path) -> int | None:
    latest: int | None = None
    for path in iterations_dir.glob("*-*.json"):
        try:
            iteration = int(path.name.split("-", 1)[0])
        except ValueError:
            continue
        latest = iteration if latest is None else max(latest, iteration)
    return latest


def build_peer_card_artifact(
    peer_input: PeerCardInput,
    draft: CardDraft,
    review: VerifierReview,
    writer_backend: str,
    writer_model: str,
    writer_effort: str,
    verifier_model: str,
    verifier_effort: str,
    iteration: int,
    verifier_service_tier: str = "",
) -> PeerCardArtifact:
    return PeerCardArtifact(
        peer_card_id=peer_input.peer_card_id,
        display_name=peer_input.peer_identity.display_name,
        card=draft.card,
        phase_boundary_note=draft.phase_boundary_note,
        source_record_ids=peer_input.peer_identity.source_record_ids,
        source_refs=collect_source_refs(peer_input.source_records),
        verifier_report=review,
        model=PeerCardModelMetadata(
            writer_backend=writer_backend,
            writer_model=writer_model,
            writer_effort=writer_effort,
            verifier_model=verifier_model,
            verifier_effort=verifier_effort,
            verifier_service_tier=verifier_service_tier,
            iterations=iteration,
            generated_at=datetime.now(timezone.utc).isoformat(),
        ),
    )


def build_peer_input(root: Path, comics_dir: Path, peer_cards_dir: Path, peer_id: str) -> PeerCardInput:
    identity_map = IdentityMapArtifact.model_validate_json(identity_map_path(peer_cards_dir).read_text())
    peer = find_peer(identity_map, peer_id)
    record_by_id = {record.record_id: record for record in identity_map.source_records}
    record_by_id.update({record.record_id: record for record in load_source_character_records(comics_dir)})
    source_records = [record_by_id[record_id] for record_id in peer.source_record_ids]
    related_non_peer_records = related_non_peer_source_records_for(peer_id, identity_map, record_by_id)
    entrypoints = build_context_entrypoints(root, comics_dir, peer_cards_dir)
    entrypoints_path = context_entrypoints_path(peer_cards_dir)
    write_json_atomic(entrypoints_path, {key: value.model_dump() for key, value in entrypoints.items()})
    related_peer_ids = related_peer_ids_for(peer_id, identity_map)
    related_identities = [find_peer(identity_map, related_id) for related_id in related_peer_ids]
    return PeerCardInput(
        peer_card_id=peer_id,
        identity_map_path=identity_map_path(peer_cards_dir).as_posix(),
        peer_identity=peer,
        source_records=source_records,
        related_non_peer_source_records=related_non_peer_records,
        related_peer_ids=related_peer_ids,
        related_peer_identities=related_identities,
        context_entrypoints_path=entrypoints_path.as_posix(),
        context_entrypoints=entrypoints,
        page_summary_paths=page_summary_paths(comics_dir, source_records),
        lothar_acceptance_rubric=lothar_acceptance_rubric() if peer_id == "lothar" else [],
    )


def mark_peer_as_non_peer_lore(
    comics_dir: Path,
    peer_cards_dir: Path,
    peer_input: PeerCardInput,
    review: PeerMaterialReview,
) -> NonPeerArtifact:
    identity_path = identity_map_path(peer_cards_dir)
    identity_map = IdentityMapArtifact.model_validate_json(identity_path.read_text())
    peer = find_peer(identity_map, peer_input.peer_card_id)
    source_record_ids = set(peer.source_record_ids)
    valid_related_peer_ids = {
        identity.peer_card_id
        for identity in identity_map.peers
        if identity.peer_card_id != peer.peer_card_id
    }
    related_peer_ids = [
        peer_id
        for peer_id in dedupe(review.related_peer_ids)
        if peer_id in valid_related_peer_ids
    ]
    reason = review.non_peer_reason.strip() or "Source record is lore evidence, not a single Honcho peer character."

    updated_records: list[SourceCharacterRecord] = []
    for record in identity_map.source_records:
        if record.record_id not in source_record_ids:
            updated_records.append(record)
            continue
        updated = record.model_copy(
            update={
                "peer_candidate": False,
                "non_peer_reason": reason,
                "related_peer_ids": related_peer_ids,
            }
        )
        updated_records.append(updated)
        update_character_source_record(comics_dir, updated)

    non_peer_by_id = {
        item.source_record_id: item
        for item in identity_map.non_peer_source_records
        if item.source_record_id not in source_record_ids
    }
    for record_id in sorted(source_record_ids):
        non_peer_by_id[record_id] = NonPeerSourceRecord(
            source_record_id=record_id,
            reason=reason,
            related_peer_ids=related_peer_ids,
        )

    identity_map = identity_map.model_copy(
        update={
            "source_records": updated_records,
            "peers": [identity for identity in identity_map.peers if identity.peer_card_id != peer.peer_card_id],
            "non_peer_source_records": [non_peer_by_id[key] for key in sorted(non_peer_by_id)],
        }
    )
    identity_map.peer_count = len(identity_map.peers)
    validate_identity_draft(
        IdentityMapDraft(peers=identity_map.peers),
        identity_map.source_records,
        identity_map.non_peer_source_records,
    )
    write_json_atomic(identity_path, identity_map.model_dump())

    return NonPeerArtifact(
        peer_card_id=peer.peer_card_id,
        source_record_ids=peer.source_record_ids,
        non_peer_reason=reason,
        related_peer_ids=related_peer_ids,
        verifier_report=review,
        generated_at=datetime.now(timezone.utc).isoformat(),
    )


def update_character_source_record(comics_dir: Path, record: SourceCharacterRecord) -> None:
    source_path = Path(record.source_file)
    if not source_path.is_absolute():
        source_path = comics_dir / "characters" / source_path.name
    data = json.loads(source_path.read_text())
    try:
        item = data[record.record_index]
    except IndexError as exc:
        raise RuntimeError(f"{record.record_id} record_index {record.record_index} is outside {source_path}") from exc
    if item.get("character_id") != record.character_id:
        raise RuntimeError(f"{record.record_id} does not match character_id at {source_path}:{record.record_index}")
    item["peer_candidate"] = record.peer_candidate
    item["non_peer_reason"] = record.non_peer_reason
    item["related_peer_ids"] = record.related_peer_ids
    write_json_atomic(source_path, data)


def related_non_peer_source_records_for(
    peer_id: str,
    identity_map: IdentityMapArtifact,
    record_by_id: dict[str, SourceCharacterRecord],
) -> list[dict[str, Any]]:
    related: list[dict[str, Any]] = []
    for non_peer in identity_map.non_peer_source_records:
        if peer_id not in non_peer.related_peer_ids:
            continue
        record = record_by_id.get(non_peer.source_record_id)
        if record is None:
            continue
        related.append(
            {
                "source_record": compact_source_record(record),
                "non_peer_reason": non_peer.reason,
                "related_peer_ids": non_peer.related_peer_ids,
            }
        )
    return related


def find_peer(identity_map: IdentityMapArtifact, peer_id: str) -> PeerIdentity:
    for peer in identity_map.peers:
        if peer.peer_card_id == peer_id:
            return peer
    raise KeyError(f"unknown peer_card_id: {peer_id}")


def build_context_entrypoints(root: Path, comics_dir: Path, peer_cards_dir: Path) -> dict[str, ContextEntrypoint]:
    def entry(path: Path, description: str) -> ContextEntrypoint:
        return ContextEntrypoint(path=path.as_posix(), description=description)

    return {
        "wiki_lore": entry(root / "lore/canon/wiki/lore.md", "High-level Jodoverse wiki lore overview. First-read orientation before digging into extracted character, volume, and page JSON."),
        "identity_map": entry(identity_map_path(peer_cards_dir), "Complete peer identity map; use this to resolve relationship targets to peer_card_id values."),
        "characters": entry(comics_dir / "characters", "Per-volume extracted structured character records."),
        "volumes": entry(comics_dir / "volumes", "Reconciled volume summaries with chronology, motifs, lore laws, and uncertainties."),
        "pages": entry(comics_dir / "pages", "Page-level summaries keyed by source image page id."),
        "link_matrix": entry(root / "lore/canon/links/matrix.md", "Wiki-shaped directional relationship matrix. Use to resolve implied lineage and relationship facts that may be fragmented in extracted JSON."),
        "peer_card_encoding_deliberation": entry(root / "council-deliberation/2026-05-04-072216.md", "Project-side peer-card writing policy: how to encode durable contradiction, spoiler boundaries, and omitted verdicts. Use as design guidance, not as canon or council mechanics."),
        "cast_docs": entry(root / "docs/mechanics/cast", "Game-side cast overlays and actor notes."),
        "lothar_cast_doc": entry(root / "docs/mechanics/cast/lothar.md", "Game-side Lothar notes; treat as design overlay, not exhaustive canon."),
        "card_seeding_docs": entry(root / "docs/architecture/actor-agents/card-seeding.md", "Project rules for actor card seeding and seed/evolution boundaries."),
        "glossary": entry(root / "docs/glossary.md", "Project definitions, including peer card constraints."),
        "narrator_card": entry(root / "lore/narrator/peer-card.md", "Narrator peer card constraints and anti-council-mode policy."),
    }


def related_peer_ids_for(peer_id: str, identity_map: IdentityMapArtifact) -> list[str]:
    if peer_id == "lothar":
        fixed = [
            "tonto",
            "the-metabaron",
            "the_metabaron",
            "the-metabaron__no-name",
            "the-metabaron__supreme",
            "aghora",
        ]
        steelhead = [peer.peer_card_id for peer in identity_map.peers if peer.peer_card_id.startswith("steelhead__")]
        seen: set[str] = set()
        existing = {item.peer_card_id for item in identity_map.peers}
        return [peer for peer in [*fixed, *steelhead] if peer in existing and not (peer in seen or seen.add(peer))]
    return []


def page_summary_paths(comics_dir: Path, source_records: list[SourceCharacterRecord]) -> dict[str, str]:
    page_ids = sorted({page_id for record in source_records for page_id in record.page_ids})
    return {page_id: (comics_dir / "pages" / f"{page_id}.json").as_posix() for page_id in page_ids}


def collect_source_refs(source_records: list[SourceCharacterRecord]) -> list[dict[str, str]]:
    refs: dict[tuple[str, str], dict[str, str]] = {}
    for record in source_records:
        for source_ref in record.source_refs:
            page_id = source_ref.get("page_id", "")
            source_image = source_ref.get("source_image", "")
            if page_id and source_image:
                refs[(page_id, source_image)] = dict(source_ref)
        if record.source_refs:
            continue
        image_sha = record.image_sha256s[0] if len(record.image_sha256s) == 1 else ""
        for page_id in record.page_ids:
            for source_image in record.source_images:
                ref = {"page_id": page_id, "source_image": source_image}
                if image_sha:
                    ref["image_sha256"] = image_sha
                refs[(page_id, source_image)] = ref
    return [refs[key] for key in sorted(refs)]


def lothar_acceptance_rubric() -> list[str]:
    return [
        "Reject if Lothar is reduced to small comic relief, a passive audience, or merely Tonto's companion.",
        "Require late Master Lothar: physically dominates/dismembers Tonto, demands formal address, and enforces narration.",
        "Require violence/status/control axis: punitive force, status-consciousness, commands obedience.",
        "Require procedural/operator competence: timing, bio-electrogram, metacraft/weapons, holographic or technical force.",
        "Require contradiction: anti-romantic machine logic but capable of curiosity, loss of faith, despair, lust, apology, and hatred.",
        "Require the wiki-level identity reveal: Lothar is in fact Steelhead after Nameless erased his memory, made him believe he was a robot, and fit Steelhead's indestructible body into armor; encode spoiler/reveal boundaries instead of treating Lothar as unrelated to Steelhead.",
        "Require boundary: Lothar can be comedic or absurd, but must not be used as a joke device that punctures every tense scene.",
        "Preserve identity-map choice: Lothar stays one peer card; phase tensions become contradictions rather than split personas.",
    ]


def run_writer(
    root: Path,
    input_path: Path,
    previous_review_path: Path | None,
    backend: str,
    model: str,
    effort: str,
    service_tier: str = "",
    timeout: int = DEFAULT_CARD_WRITER_TIMEOUT,
) -> CardDraft:
    model, effort = resolve_writer_model_effort(backend, model, effort)
    prompt = writer_prompt(input_path=input_path, previous_review_path=previous_review_path)
    if backend == "claude":
        raw = run_claude_json(
            prompt=prompt,
            schema_or_model=CardDraft,
            model=model,
            effort=effort,
            timeout=timeout,
            cwd=root,
            add_dir=root,
        )
    elif backend == "codex-oauth":
        raw = run_codex_oauth_json(
            root=root,
            prompt=prompt,
            schema_or_model=CardDraft,
            model=model,
            effort=effort,
            timeout=timeout,
            service_tier=service_tier,
        )
    else:
        raise ValueError(f"unknown writer backend: {backend}")
    return CardDraft.model_validate_json(raw)


def resolve_writer_model_effort(backend: str, model: str, effort: str) -> tuple[str, str]:
    if backend != "codex-oauth":
        return model, effort
    if model == DEFAULT_CARD_WRITER_MODEL and "SANS_NOM_PEER_CARD_WRITER_MODEL" not in os.environ:
        model = DEFAULT_CODEX_OAUTH_WRITER_MODEL
    if effort == DEFAULT_CARD_WRITER_EFFORT and "SANS_NOM_PEER_CARD_WRITER_EFFORT" not in os.environ:
        effort = DEFAULT_CODEX_OAUTH_WRITER_EFFORT
    return model, effort


def run_codex_oauth_json(
    root: Path,
    prompt: str,
    schema_or_model: Any,
    model: str,
    effort: str,
    timeout: int,
    service_tier: str = "",
) -> str:
    payload = {
        "body": {
            "model": model,
            "input": [
                {
                    "role": "user",
                    "content": prompt,
                }
            ],
            "reasoning": {"effort": effort},
            "text": {
                "format": {
                    "type": "json_schema",
                    "name": schema_or_model.__name__,
                    "schema": schema_for(schema_or_model),
                    "strict": True,
                }
            },
        }
    }
    service_tier = resolve_codex_oauth_service_tier(service_tier)
    if service_tier:
        payload["body"]["service_tier"] = service_tier

    fd, request_name = tempfile.mkstemp(prefix="sans-nom-codex-oauth-", suffix=".json")
    request_path = Path(request_name)
    try:
        with os.fdopen(fd, "w") as f:
            json.dump(payload, f, ensure_ascii=False, indent=2)
            f.write("\n")
        helper = os.getenv("SANS_NOM_CODEX_OAUTH_BIN")
        if helper:
            cmd = [
                helper,
                "--input",
                request_path.as_posix(),
                "--timeout",
                f"{timeout}s",
                "--tool-root",
                root.as_posix(),
            ]
        else:
            cmd = [
                "go",
                "run",
                "./cmd/codex-responses-json",
                "--input",
                request_path.as_posix(),
                "--timeout",
                f"{timeout}s",
                "--tool-root",
                root.as_posix(),
            ]
        auth_path = os.getenv("SANS_NOM_CODEX_AUTH_PATH")
        if auth_path:
            cmd.extend(["--auth-path", auth_path])
        try:
            result = run_command_captured(cmd, cwd=root, timeout=timeout + 30)
        except subprocess.CalledProcessError as exc:
            stderr = (exc.stderr or exc.stdout or "").strip()[-4000:]
            raise RuntimeError(f"codex oauth writer failed with exit code {exc.returncode}: {stderr}") from exc
        except subprocess.TimeoutExpired as exc:
            raise RuntimeError(f"codex oauth writer timed out after {timeout} seconds") from exc
        return result.stdout
    finally:
        request_path.unlink(missing_ok=True)


def resolve_codex_oauth_service_tier(service_tier: str) -> str:
    tier = service_tier.strip()
    if not tier:
        tier = os.getenv("SANS_NOM_CODEX_OAUTH_SERVICE_TIER", "").strip()
    return CODEX_OAUTH_SERVICE_TIER_ALIASES.get(tier.lower(), tier)


def resolve_codex_cli_service_tier(service_tier: str) -> str:
    tier = service_tier.strip()
    return CODEX_CLI_SERVICE_TIER_ALIASES.get(tier.lower(), tier)


def writer_prompt(input_path: Path, previous_review_path: Path | None) -> str:
    revision = ""
    if previous_review_path is not None:
        revision = f"""

The previous verifier review is at:
{previous_review_path.as_posix()}

Read that review and revise the card until the blocking issues are fixed.
Treat the review as a narrow change list. If it says the rest of the card is sound, keep the structure conservative and do not add replacement claims unless the source records directly support them.
"""
    return f"""You are the creative Writer for a Honcho peer-card generation loop.

Task:
- Read the full peer-card input JSON from:
  {input_path.as_posix()}
- Before digging into the deep extracted dataset, read these paths from `context_entrypoints` in this order:
  1. `wiki_lore.path` for first-pass continuity orientation.
  2. `link_matrix.path` for wiki-shaped directional relationship facts.
  3. `peer_card_encoding_deliberation.path` for card-writing policy on durable contradiction, spoiler boundaries, and omitted verdicts.
- Treat the wiki lore and link matrix as orientation and relationship scaffolding. Use them to resolve implied facts that page JSON may fragment, then verify against source records/page/volume refs when possible.
- Treat `peer_card_encoding_deliberation` as design guidance only, never as in-world canon and never as council/game mechanics.
- The target is exactly the `peer_identity` in the input JSON. Do not merge it with same-name, title-similar, later-phase, earlier-phase, or wiki-level characters unless their record ids are present in `peer_identity.source_record_ids`.
- The card must be source-bounded to `source_records`. For singleton or thin peers, write a smaller precise card; do not inflate it into a full-arc biography from wiki orientation.
- `related_non_peer_source_records` are lore evidence only. Use them for lineage, aliases, and witnessed scene facts when they name this peer, but never create or imply a separate Honcho peer for a group/faction/plural record.
- Produce one Honcho-native base peer card for that peer.
- Return only JSON matching the provided schema. No markdown.

Card contract:
- `card` is exactly the Honcho payload: list[str], max 40 facts.
- Every fact must start with one allowed prefix:
  {", ".join(ALLOWED_CARD_PREFIXES)}
- Facts must be durable, atomic, behaviorally useful, and grounded in the source bundle.
- For contradiction, encode the character's internal organizing sentence, irreducible refusal, load-bearing pressure behavior, or hunger-shape. Do not merely label the contradiction from outside.
- For meaningful characters, include what the character refuses to become or destroy themselves before becoming when the source supports it. Do not reduce people to function labels like mother, vessel, heir, servant, witness, victim, or origin point.
- Spoiler-bound facts should protect sequenced scenes, names, omissions, or acts whose early disclosure would make the narrator perform toward them.
- Leave out verdicts, vindication, final resolution, exact transformative lines, and the character's final self-judgment. The card should give the narrator the wound-shape and conditions under which the character becomes inevitable, not the eleven words or a map of the ending.
- Source refs, paths, filenames, page ids, and hashes stay out of card facts.
- Relationship facts should reference resolved peer ids when useful, e.g. `RELATIONSHIP: tonto (Tonto): ...`.
- Relationship targets must come from the current input's source records, `related_peer_identities`, or identity-map resolution for names directly supported by this peer's source records. Do not add wiki/matrix relationships that belong to a different split identity.
- Preserve split-persona identity-map decisions. If the input says this peer remains one identity, encode phase tension as contradiction, not as a split.
- Do not introduce council mode, all-cast fanout, resurrection-roster assumptions, or non-witnessed omniscience.
- Use the link matrix as source support for implied lineage/relationship facts, never as game mechanics.

For Lothar specifically, obey the `lothar_acceptance_rubric` in the input JSON. The card must not flatten him into a small joke companion.
{revision}
"""


def run_peer_material_verifier(
    root: Path,
    input_path: Path,
    model: str,
    effort: str,
    service_tier: str,
    timeout: int = DEFAULT_PEER_MATERIAL_TIMEOUT,
) -> PeerMaterialReview:
    service_tier = resolve_codex_cli_service_tier(service_tier)
    schema_path = write_schema_tempfile(PeerMaterialReview)
    tmp_out = input_path.with_suffix(".peer-material.tmp.json")
    try:
        cmd = [
            "codex",
            "exec",
            "--ephemeral",
            "--dangerously-bypass-approvals-and-sandbox",
            "-C",
            root.as_posix(),
            "-m",
            model,
            "-c",
            f'model_reasoning_effort="{effort}"',
            "-c",
            f'service_tier="{service_tier}"',
            "--output-schema",
            schema_path.as_posix(),
            "-o",
            tmp_out.as_posix(),
            peer_material_prompt(input_path),
        ]
        try:
            run_command_captured(cmd, cwd=root, timeout=timeout)
        except subprocess.CalledProcessError as exc:
            tmp_out.unlink(missing_ok=True)
            stderr = (exc.stderr or exc.stdout or "").strip()[-4000:]
            raise RuntimeError(f"codex peer-material verifier failed with exit code {exc.returncode}: {stderr}") from exc
        except subprocess.TimeoutExpired as exc:
            tmp_out.unlink(missing_ok=True)
            raise RuntimeError(f"codex peer-material verifier timed out after {timeout} seconds") from exc
        return PeerMaterialReview.model_validate_json(tmp_out.read_text())
    finally:
        schema_path.unlink(missing_ok=True)
        tmp_out.unlink(missing_ok=True)


def peer_material_prompt(input_path: Path) -> str:
    return f"""You are the verifier-first peer-material reviewer for the Honcho peer-card pipeline.

Read the input bundle:
{input_path.as_posix()}

Return only JSON matching the provided schema. Do not edit files.

Task:
- Decide whether the target `peer_identity` is worth a base Honcho peer card before the expensive Writer loop runs.
- This is not card-writing and not card-polishing. It is a gate for "is this a usable peer persona?"
- If it is not peer-card material, set `peer_card_material=false`, `should_mark_non_peer_lore=true`, write a concise `non_peer_reason`, and list valid `related_peer_ids` that should inherit the lore evidence.

Mark as non-peer lore when the source bundle is mainly:
- a title, rumor, scar-origin hook, unidentified figure, object, place, one-line role, anonymous crowd, or plural fact bundle;
- an alias/subtitle/reference to another peer rather than a distinct persona;
- a source-thin record whose only value is contextual lore for another peer;
- something that would force the Writer to invent traits just to pass review.

Keep as peer-card material when the bundle supports:
- a coherent actor, narrator, faction office, or intentional group-role peer with durable behavior;
- a thin but distinct single actor whose action, relationship, voice, or boundary is useful in scenes;
- a deliberately split phase/persona from the identity map;
- project-approved group-role peers such as Shabda-Oud witches or mother superiors.

Important:
- Do not demote major dynasty figures, named phase-split personas, or source-thin characters only because their cards will be small.
- Do not invent related peers. Use `related_peer_identities`, relationship text, source records, or clear identity-map ids.
- Treat title/mystery-hook records like "the invincible warrior" as non-peer lore when they only preserve a scar-origin or frame-story clue; attach that evidence to the real related peers instead.
"""


def run_verifier(
    root: Path,
    input_path: Path,
    draft_path: Path,
    model: str,
    effort: str,
    service_tier: str,
    local_errors: list[str],
    timeout: int = DEFAULT_CARD_VERIFIER_TIMEOUT,
) -> VerifierReview:
    service_tier = resolve_codex_cli_service_tier(service_tier)
    schema_path = write_schema_tempfile(VerifierReview)
    tmp_out = draft_path.with_suffix(".verifier.tmp.json")
    try:
        prompt = verifier_prompt(
            input_path=input_path,
            draft_path=draft_path,
            local_errors=local_errors,
        )
        cmd = [
            "codex",
            "exec",
            "--ephemeral",
            "--dangerously-bypass-approvals-and-sandbox",
            "-C",
            root.as_posix(),
            "-m",
            model,
            "-c",
            f'model_reasoning_effort="{effort}"',
            "-c",
            f'service_tier="{service_tier}"',
            "--output-schema",
            schema_path.as_posix(),
            "-o",
            tmp_out.as_posix(),
            prompt,
        ]
        try:
            run_command_captured(cmd, cwd=root, timeout=timeout)
        except subprocess.CalledProcessError as exc:
            tmp_out.unlink(missing_ok=True)
            stderr = (exc.stderr or exc.stdout or "").strip()[-4000:]
            raise RuntimeError(f"codex verifier failed with exit code {exc.returncode}: {stderr}") from exc
        except subprocess.TimeoutExpired as exc:
            tmp_out.unlink(missing_ok=True)
            raise RuntimeError(f"codex verifier timed out after {timeout} seconds") from exc
        return VerifierReview.model_validate_json(tmp_out.read_text())
    finally:
        schema_path.unlink(missing_ok=True)
        tmp_out.unlink(missing_ok=True)


def verifier_prompt(input_path: Path, draft_path: Path, local_errors: list[str]) -> str:
    return f"""You are the strict Verifier for a Honcho peer-card generation loop.

Read these files:
- Input bundle: {input_path.as_posix()}
- Writer draft: {draft_path.as_posix()}

Return only JSON matching the provided schema. Do not edit files.

Review duties:
- Validate the Honcho card contract: list[str], max 40 facts, allowed prefixes only, no markdown, no source refs in facts, no duplicates.
- Reject unsupported biography, generic sci-fi flattening, and facts that ignore source knowledge boundaries.
- Reject cards that use wiki lore or link-matrix orientation to override the target `peer_identity` or import full-arc facts from a different split/same-name peer.
- Reject cards that flatten contradiction into outside labels without an internal organizing sentence, irreducible refusal, pressure behavior, or hunger-shape when the source bundle supports that depth.
- Reject meaningful cards that reduce a character to function, role, or relationship while omitting what they refuse to become when the source bundle supports that refusal.
- Reject cards that encode final verdicts, vindication, resolved endings, fixed self-judgment, or exact transformative lines as Honcho facts; those belong to play, not the base card.
- For split persona / phase peer cards, make sure the card follows the identity-map entry and relationships point at the right peer ids.
- For Lothar, enforce every item in `lothar_acceptance_rubric`; reject any card that makes him only small comic relief or only a passive audience.
- For source-thin minor characters, do not push the Writer to invent lore just to make the card feel important. If the card is format-valid, source-grounded, and honest about thin boundaries, accept it and put a short note in `manual_review_notes`.
- Return accepted=true only when the card is ready to stage.
- Use blocked_source_insufficient=true only when the source bundle cannot support a useful card at all.

Deterministic local validation errors already found:
{json.dumps(local_errors, ensure_ascii=False, indent=2)}
"""


def validate_card_contract(card: list[str]) -> list[str]:
    errors: list[str] = []
    if not card:
        return ["card is empty"]
    if len(card) > 40:
        errors.append(f"card has {len(card)} facts; max is 40")

    seen: set[str] = set()
    forbidden_source_markers = (
        "source_ref",
        "source refs",
        "page_id",
        "image_sha",
        "lore/canon",
        "external/comics",
        ".json",
        ".jpg",
        ".jpeg",
        ".png",
    )
    forbidden_markdown_prefixes = ("- ", "* ", "#", "```", ">")
    for index, fact in enumerate(card, start=1):
        if not isinstance(fact, str) or not fact.strip():
            errors.append(f"fact {index} is empty or not a string")
            continue
        stripped = fact.strip()
        if stripped != fact:
            errors.append(f"fact {index} has leading/trailing whitespace")
        if stripped in seen:
            errors.append(f"duplicate fact {index}: {stripped!r}")
        seen.add(stripped)
        if not stripped.startswith(ALLOWED_CARD_PREFIXES):
            errors.append(f"fact {index} uses an unsupported prefix: {stripped!r}")
        if stripped.startswith(forbidden_markdown_prefixes):
            errors.append(f"fact {index} contains markdown formatting")
        lowered = stripped.lower()
        if any(marker in lowered for marker in forbidden_source_markers):
            errors.append(f"fact {index} embeds source refs or paths")
    return errors


def validate_lothar_card_rubric(card: list[str]) -> list[str]:
    joined = "\n".join(card).lower()
    errors: list[str] = []

    if "lothar" not in joined:
        errors.append("Lothar card must name Lothar.")
    if "master lothar" not in joined and "sir" not in joined:
        errors.append("Lothar card must capture late Master Lothar/status reversal.")
    if not has_any(joined, ("dismember", "dismembers", "tear", "rips", "rip", "sever")):
        errors.append("Lothar card must capture that he physically dismembers or tears apart Tonto.")
    if not ("tonto" in joined and has_any(joined, ("demand", "sir", "coerc", "obedience", "enforces narration", "forces narration"))):
        errors.append("Lothar card must capture coercive control over Tonto and narration.")
    if "repairer" in joined or "forced repair" in joined:
        errors.append("Lothar card must not claim Tonto is Lothar's repairer or that Lothar uses forced repair as coercion.")
    if "identity-circuit" in joined or "identity circuit" in joined or lothar_uses_circuit_repair_threat(card):
        errors.append("Lothar card must not attribute identity-circuit or circuit-repair threats to Lothar.")
    if not has_any(joined, ("bio-electrogram", "metacraft", "holographic", "solidification", "weapon", "timing", "operator", "technical")):
        errors.append("Lothar card must capture procedural/operator competence.")
    if "steelhead" not in joined:
        errors.append("Lothar card must capture the wiki-level reveal that Lothar is Steelhead.")
    if "steelhead" in joined and not has_any(joined, ("memory", "amnes", "believe", "robot", "armor", "armour", "indestructible body")):
        errors.append("Lothar/Steelhead reveal must include the memory-erasure/robot-belief/armored-body boundary.")

    contradiction_hits = sum(
        has_any(joined, options)
        for options in (
            ("anti-romantic", "machine logic", "hard rationalist", "love as"),
            ("curiosity", "curious"),
            ("faith", "loss of faith"),
            ("despair", "suicidal"),
            ("lust",),
            ("apology", "apologizes", "apologise", "sorry"),
            ("hatred", "hate"),
        )
    )
    if contradiction_hits < 5:
        errors.append("Lothar card must capture his machine-logic/emotional contradiction with enough specificity.")

    boundary_facts = [fact.lower() for fact in card if fact.startswith("BOUNDARY:")]
    if not boundary_facts:
        errors.append("Lothar card needs a BOUNDARY fact.")
    elif not any(("comic" in fact or "absurd" in fact or "joke" in fact) and "not" in fact for fact in boundary_facts):
        errors.append("Lothar boundary must say he can be comic/absurd without becoming a joke device.")

    if "small comic relief companion" in joined and len(card) < 8:
        errors.append("Lothar is reduced to small comic relief companion.")
    return errors


def has_any(text: str, needles: tuple[str, ...]) -> bool:
    return any(needle in text for needle in needles)


def lothar_uses_circuit_repair_threat(card: list[str]) -> bool:
    for fact in card:
        lowered = fact.lower()
        if "circuit repair" not in lowered:
            continue
        if "may threaten" in lowered or "can threaten" in lowered or "uses threats" in lowered:
            return True
        if "to obtain compliance" in lowered or "as coercion" in lowered or "coercive vocabulary" in lowered:
            return True
    return False


def write_schema_tempfile(schema_or_model: Any) -> Path:
    fd, name = tempfile.mkstemp(prefix="sans-nom-peer-card-schema-", suffix=".json")
    path = Path(name)
    with os.fdopen(fd, "w") as f:
        json.dump(schema_for(schema_or_model), f, ensure_ascii=False)
    return path


@contextmanager
def tempfile_json(payload: dict[str, Any]) -> Iterator[Path]:
    fd, name = tempfile.mkstemp(prefix="sans-nom-peer-identity-", suffix=".json")
    path = Path(name)
    try:
        with os.fdopen(fd, "w") as f:
            json.dump(payload, f, ensure_ascii=False, indent=2, sort_keys=True)
            f.write("\n")
        yield path
    finally:
        path.unlink(missing_ok=True)


def identity_map_prompt(
    input_path: Path,
    previous_output: str | None,
    previous_errors: list[str],
) -> str:
    repair = ""
    if previous_output is not None:
        repair = f"""

Your previous output did not validate. Fix it.
Validation errors:
{json.dumps(previous_errors, ensure_ascii=False, indent=2)}

Previous output:
{previous_output}
"""
    return f"""You are resolving extracted Metabarons character records into Honcho peer-card identities.

Goal:
- Produce a peer identity map, not peer cards.
- Each peer identity will later become one Honcho peer card.
- Honcho peers must represent coherent writable personas, not merely shared names.

Critical rules:
- Every source record_id must appear in exactly one peer identity.
- Do not invent source record_ids.
- Split same-name records when phases/arcs would fight inside one Honcho card: child vs adult, warrior vs sealed/ruined, pre-trauma vs post-trauma, living actor vs remembered/dead story object, title/role vs specific person.
- Continuity of body/name is not enough to merge. Honcho-card coherence wins: if one card would need contradictory TRAIT/VOICE/INSTRUCTION facts, split it.
- Long lifetime arcs should split when the character's usable actor/writer persona changes drastically. Examples to watch for: Steelhead as warrior heir vs Melmoth-touched/ruined/sealed figure; Honorata living priestess vs reincarnated body; different people sharing "Oda".
- Merge records when they are the same stable persona across volumes and can share one durable card.
- You may merge across different character_id values when aliases clearly describe the same persona.
- Use lowercase slug peer_card_id values, with phase suffixes when split, e.g. steelhead__warrior, steelhead__late-sealed.
- Avoid council mechanics, all-cast fanout, resurrected roster assumptions, or anything about invoking these peers together.
- Return only JSON matching the provided schema.

Read the input JSON from this file:
{input_path.as_posix()}

The file contains only duplicate character_id groups and alias-linked records that require judgment. Singleton records are already handled outside this model call.
{repair}
"""


def compact_source_record(record: SourceCharacterRecord) -> dict[str, Any]:
    return {
        "record_id": record.record_id,
        "character_id": record.character_id,
        "peer_candidate": record.peer_candidate,
        "non_peer_reason": record.non_peer_reason,
        "related_peer_ids": record.related_peer_ids,
        "source_file": record.source_file,
        "aliases": record.aliases,
        "volumes": record.volumes,
        "page_ids": record.page_ids,
        "appearances": record.appearances,
        "traits": record.traits,
        "relationships": record.relationships,
        "witnessed_facts": record.witnessed_facts,
        "knowledge_boundaries": record.knowledge_boundaries,
        "important_actions": record.important_actions,
        "voice_notes": record.voice_notes,
    }


def normalize_peer_ids(draft: IdentityMapDraft) -> None:
    used: Counter[str] = Counter()
    for peer in draft.peers:
        base = slugify_peer_id(peer.peer_card_id)
        used[base] += 1
        peer.peer_card_id = base if used[base] == 1 else f"{base}-{used[base]}"


def validate_identity_draft(
    draft: IdentityMapDraft,
    source_records: list[SourceCharacterRecord],
    non_peer_source_records: list[NonPeerSourceRecord] | None = None,
) -> None:
    errors: list[str] = []
    valid_ids = {record.record_id for record in source_records}
    non_peer_source_records = non_peer_source_records or []
    non_peer_ids = {record.source_record_id for record in non_peer_source_records}
    invalid_non_peer_ids = sorted(non_peer_ids - valid_ids)
    candidate_ids = valid_ids - non_peer_ids
    seen: list[str] = []
    peer_ids: list[str] = []
    for index, peer in enumerate(draft.peers):
        peer_ids.append(peer.peer_card_id)
        if not peer.source_record_ids:
            errors.append(f"peer {index} {peer.peer_card_id} has no source_record_ids")
        if peer.peer_card_id != slugify_peer_id(peer.peer_card_id):
            errors.append(f"peer {index} has non-slug peer_card_id {peer.peer_card_id!r}")
        for record_id in peer.source_record_ids:
            if record_id not in valid_ids:
                errors.append(f"peer {peer.peer_card_id} references unknown record_id {record_id}")
            if record_id in non_peer_ids:
                errors.append(f"peer {peer.peer_card_id} references non-peer source record {record_id}")
            seen.append(record_id)

    duplicate_peer_ids = sorted([peer_id for peer_id, count in Counter(peer_ids).items() if count > 1])
    separator_collisions = {
        key: sorted(values)
        for key, values in separator_key_buckets(peer_ids).items()
        if len(set(values)) > 1
    }
    duplicate_record_ids = sorted([record_id for record_id, count in Counter(seen).items() if count > 1])
    missing_record_ids = sorted(candidate_ids - set(seen))
    if invalid_non_peer_ids:
        errors.append(f"non_peer_source_records reference unknown record_ids: {invalid_non_peer_ids[:20]}")
    if duplicate_peer_ids:
        errors.append(f"duplicate peer_card_id values: {duplicate_peer_ids}")
    if separator_collisions:
        errors.append(f"peer_card_id values differ only by '-'/'_' separators: {separator_collisions}")
    if duplicate_record_ids:
        errors.append(f"duplicate source_record_ids: {duplicate_record_ids[:20]}")
    if missing_record_ids:
        errors.append(f"missing source_record_ids: {missing_record_ids[:20]} ({len(missing_record_ids)} total)")
    if errors:
        raise IdentityValidationError(errors)


def separator_key_buckets(peer_ids: list[str]) -> dict[str, list[str]]:
    buckets: dict[str, list[str]] = {}
    for peer_id in peer_ids:
        buckets.setdefault(separator_key(peer_id), []).append(peer_id)
    return buckets


def separator_key(peer_id: str) -> str:
    return re.sub(r"[-_]+", "-", peer_id)


def identity_status(peer_cards_dir: Path) -> dict[str, Any]:
    path = identity_map_path(peer_cards_dir)
    if not path.exists():
        return {
            "identity_map": "missing",
            "input_record_count": 0,
            "raw_character_id_count": 0,
            "peer_count": 0,
        }
    artifact = IdentityMapArtifact.model_validate_json(path.read_text())
    return {
        "identity_map": path.as_posix(),
        "input_record_count": artifact.input_record_count,
        "raw_character_id_count": artifact.raw_character_id_count,
        "peer_count": artifact.peer_count,
        "iterations": artifact.model.iterations,
        "writer_model": artifact.model.writer_model,
        "writer_effort": artifact.model.writer_effort,
    }


def validate_peer_card_outputs(peer_cards_dir: Path) -> list[str]:
    errors: list[str] = []
    path = identity_map_path(peer_cards_dir)
    if not path.exists():
        return [f"missing {path}"]
    try:
        artifact = IdentityMapArtifact.model_validate_json(path.read_text())
        validate_identity_draft(
            IdentityMapDraft(peers=artifact.peers),
            artifact.source_records,
            artifact.non_peer_source_records,
        )
        if artifact.input_record_count != len(artifact.source_records):
            errors.append("input_record_count does not match source_records length")
        if artifact.raw_character_id_count != len({record.character_id for record in artifact.source_records}):
            errors.append("raw_character_id_count does not match source_records")
        if artifact.peer_count != len(artifact.peers):
            errors.append("peer_count does not match peers length")
    except (ValidationError, IdentityValidationError) as exc:
        if isinstance(exc, IdentityValidationError):
            errors.extend(exc.errors)
        else:
            errors.append(str(exc))

    for card_path in sorted((peer_cards_dir / "cards" / "accepted").glob("*.json")):
        try:
            card_artifact = PeerCardArtifact.model_validate_json(card_path.read_text())
        except ValidationError as exc:
            errors.append(f"{card_path}: {exc}")
            continue
        expected_peer_id = card_path.stem
        if card_artifact.peer_card_id != expected_peer_id:
            errors.append(f"{card_path}: peer_card_id {card_artifact.peer_card_id!r} does not match artifact filename {expected_peer_id!r}")
        for error in validate_card_contract(card_artifact.card):
            errors.append(f"{card_path}: {error}")
        if card_artifact.peer_card_id == "lothar":
            for error in validate_lothar_card_rubric(card_artifact.card):
                errors.append(f"{card_path}: {error}")
    for non_peer_path in sorted((peer_cards_dir / "cards" / "non-peer").glob("*.json")):
        try:
            non_peer_artifact = NonPeerArtifact.model_validate_json(non_peer_path.read_text())
        except ValidationError as exc:
            errors.append(f"{non_peer_path}: {exc}")
            continue
        expected_peer_id = non_peer_path.stem
        if non_peer_artifact.peer_card_id != expected_peer_id:
            errors.append(f"{non_peer_path}: peer_card_id {non_peer_artifact.peer_card_id!r} does not match artifact filename {expected_peer_id!r}")
    return errors


def run_claude_json(
    prompt: str,
    schema_or_model: Any,
    model: str,
    effort: str,
    timeout: int,
    cwd: Path | None = None,
    add_dir: Path | None = None,
) -> str:
    schema = json.dumps(schema_for(schema_or_model), ensure_ascii=False)
    cmd = [
        "claude",
        "--model",
        model,
        "-p",
        "--no-session-persistence",
        "--permission-mode",
        "bypassPermissions",
        "--output-format",
        "json",
        "--effort",
        effort,
        "--json-schema",
        schema,
    ]
    if add_dir is not None:
        cmd[1:1] = ["--add-dir", add_dir.as_posix()]
    try:
        result = run_command_captured(
            cmd,
            cwd=cwd,
            input=prompt,
            timeout=timeout,
        )
    except subprocess.CalledProcessError as exc:
        stderr = (exc.stderr or exc.stdout or "").strip()
        raise RuntimeError(f"claude failed with exit code {exc.returncode}: {stderr}") from exc
    except subprocess.TimeoutExpired as exc:
        raise RuntimeError(f"claude timed out after {timeout} seconds") from exc
    return extract_claude_json(result.stdout)


def run_command_captured(
    cmd: list[str],
    cwd: Path | None,
    timeout: int,
    input: str | None = None,
) -> subprocess.CompletedProcess[str]:
    install_shutdown_handlers()
    process = subprocess.Popen(
        cmd,
        cwd=cwd,
        text=True,
        stdin=subprocess.PIPE if input is not None else None,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        start_new_session=True,
    )
    with ACTIVE_PROCESS_LOCK:
        ACTIVE_PROCESSES.add(process)
    try:
        stdout, stderr = process.communicate(input=input, timeout=timeout)
    except subprocess.TimeoutExpired as exc:
        terminate_process_group(process)
        stdout, stderr = process.communicate()
        raise subprocess.TimeoutExpired(cmd, timeout, output=stdout, stderr=stderr) from exc
    except BaseException:
        terminate_process_group(process)
        raise
    finally:
        with ACTIVE_PROCESS_LOCK:
            ACTIVE_PROCESSES.discard(process)
    result = subprocess.CompletedProcess(cmd, process.returncode, stdout, stderr)
    if process.returncode:
        raise subprocess.CalledProcessError(process.returncode, cmd, output=stdout, stderr=stderr)
    return result


def install_shutdown_handlers() -> None:
    global SIGNAL_HANDLERS_INSTALLED
    if SIGNAL_HANDLERS_INSTALLED or threading.current_thread() is not threading.main_thread():
        return
    signal.signal(signal.SIGTERM, handle_shutdown_signal)
    signal.signal(signal.SIGINT, handle_shutdown_signal)
    SIGNAL_HANDLERS_INSTALLED = True


def handle_shutdown_signal(signum: int, frame: Any) -> None:
    terminate_active_process_groups()
    raise KeyboardInterrupt(f"received signal {signum}")


def terminate_active_process_groups() -> None:
    with ACTIVE_PROCESS_LOCK:
        processes = list(ACTIVE_PROCESSES)
    for process in processes:
        terminate_process_group(process)


def terminate_process_group(process: subprocess.Popen[str]) -> None:
    try:
        os.killpg(process.pid, signal.SIGTERM)
    except ProcessLookupError:
        return
    try:
        process.wait(timeout=5)
    except subprocess.TimeoutExpired:
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            return
        process.wait()


def dedupe(values: list[str]) -> list[str]:
    seen: set[str] = set()
    out: list[str] = []
    for value in values:
        if value in seen:
            continue
        seen.add(value)
        out.append(value)
    return out


def slugify_peer_id(value: str) -> str:
    slug = value.lower().strip()
    slug = re_sub_peer_id(slug)
    slug = slug.strip("-_")
    return slug or "unknown-peer"


def re_sub_peer_id(value: str) -> str:
    import re

    value = re.sub(r"\s+", "-", value)
    value = re.sub(r"[^a-z0-9_-]+", "-", value)
    value = re.sub(r"-{2,}", "-", value)
    return value


if __name__ == "__main__":
    raise SystemExit(main())
