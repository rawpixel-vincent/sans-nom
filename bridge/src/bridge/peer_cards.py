from __future__ import annotations

import argparse
import json
import os
import subprocess
import tempfile
from collections import Counter
from contextlib import contextmanager
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Iterator, Literal

from pydantic import BaseModel, Field, ValidationError

from .comic_lore import CharacterLore, VolumeSummary, extract_claude_json, schema_for, write_json_atomic
from .env import load_project_env

load_project_env()

DEFAULT_COMICS_DIR = Path("lore/canon/comics")
DEFAULT_PEER_CARDS_DIR = Path("lore/canon/comics/peer-cards")
DEFAULT_IDENTITY_MODEL = "claude-sonnet-4-6"
DEFAULT_IDENTITY_EFFORT = "low"


class SourceCharacterRecord(BaseModel):
    record_id: str
    character_id: str
    source_file: str
    record_index: int
    aliases: list[str] = Field(default_factory=list)
    volumes: list[int] = Field(default_factory=list)
    page_ids: list[str] = Field(default_factory=list)
    source_images: list[str] = Field(default_factory=list)
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


class IdentityValidationError(ValueError):
    def __init__(self, errors: list[str]) -> None:
        super().__init__("\n".join(errors))
        self.errors = errors


def main(argv: list[str] | None = None) -> int:
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

    status = sub.add_parser("status", help="Show identity-map status.")
    status.add_argument("--json", action="store_true")

    validate = sub.add_parser("validate", help="Validate peer-card staging artifacts.")
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


def generate_identity_map(
    comics_dir: Path,
    writer_model: str,
    writer_effort: str,
    max_iterations: int,
) -> IdentityMapArtifact:
    source_records = load_source_character_records(comics_dir)
    volumes = load_volume_context(comics_dir)
    singleton_peers, duplicate_records = split_singletons_and_duplicates(source_records)
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
            normalize_peer_ids(draft)
            validate_identity_draft(draft, source_records)
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
        )

    raise RuntimeError(f"identity-map generation failed after {max_iterations} iterations: {previous_errors}")


def split_singletons_and_duplicates(
    source_records: list[SourceCharacterRecord],
) -> tuple[list[PeerIdentity], list[SourceCharacterRecord]]:
    counts = Counter(record.character_id for record in source_records)
    singletons: list[PeerIdentity] = []
    duplicates: list[SourceCharacterRecord] = []
    for record in source_records:
        if counts[record.character_id] > 1:
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
        "task": "Resolve only duplicate character_id groups into Honcho peer-card identities. Singleton records are handled deterministically outside this model call.",
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
            records.append(
                SourceCharacterRecord(
                    record_id=f"{source_path.name}#{index:03d}:{character.character_id}",
                    character_id=character.character_id,
                    source_file=source_path.as_posix(),
                    record_index=index,
                    aliases=dedupe(character.aliases),
                    volumes=sorted(set(character.volumes)),
                    page_ids=page_ids,
                    source_images=source_images,
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

The file contains only duplicate character_id groups that require judgment. Singleton records are already handled outside this model call.
{repair}
"""


def compact_source_record(record: SourceCharacterRecord) -> dict[str, Any]:
    return {
        "record_id": record.record_id,
        "character_id": record.character_id,
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


def validate_identity_draft(draft: IdentityMapDraft, source_records: list[SourceCharacterRecord]) -> None:
    errors: list[str] = []
    valid_ids = {record.record_id for record in source_records}
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
            seen.append(record_id)

    duplicate_peer_ids = sorted([peer_id for peer_id, count in Counter(peer_ids).items() if count > 1])
    duplicate_record_ids = sorted([record_id for record_id, count in Counter(seen).items() if count > 1])
    missing_record_ids = sorted(valid_ids - set(seen))
    if duplicate_peer_ids:
        errors.append(f"duplicate peer_card_id values: {duplicate_peer_ids}")
    if duplicate_record_ids:
        errors.append(f"duplicate source_record_ids: {duplicate_record_ids[:20]}")
    if missing_record_ids:
        errors.append(f"missing source_record_ids: {missing_record_ids[:20]} ({len(missing_record_ids)} total)")
    if errors:
        raise IdentityValidationError(errors)


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
        validate_identity_draft(IdentityMapDraft(peers=artifact.peers), artifact.source_records)
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
    return errors


def run_claude_json(
    prompt: str,
    schema_or_model: Any,
    model: str,
    effort: str,
    timeout: int,
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
    try:
        result = subprocess.run(
            cmd,
            check=True,
            text=True,
            capture_output=True,
            input=prompt,
            timeout=timeout,
        )
    except subprocess.CalledProcessError as exc:
        stderr = (exc.stderr or exc.stdout or "").strip()
        raise RuntimeError(f"claude failed with exit code {exc.returncode}: {stderr}") from exc
    except subprocess.TimeoutExpired as exc:
        raise RuntimeError(f"claude timed out after {timeout} seconds") from exc
    return extract_claude_json(result.stdout)


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
