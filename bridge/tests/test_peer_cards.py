from __future__ import annotations

import json
from pathlib import Path

import pytest

from bridge.peer_cards import (
    IdentityMapDraft,
    IdentityValidationError,
    PeerIdentity,
    SourceCharacterRecord,
    identity_map_prompt,
    identity_status,
    load_source_character_records,
    split_singletons_and_duplicates,
    validate_identity_draft,
    validate_peer_card_outputs,
)


def write_character_file(path: Path, items: list[dict[str, object]]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(items))


def character_item(character_id: str, aliases: list[str], volume: int) -> dict[str, object]:
    return {
        "character_id": character_id,
        "aliases": aliases,
        "volumes": [volume],
        "appearances": [f"volume {volume} appearance"],
        "traits": [],
        "relationships": [],
        "witnessed_facts": [],
        "knowledge_boundaries": [],
        "important_actions": [],
        "voice_notes": [],
        "source_refs": [
            {
                "page_id": f"v{volume}p1",
                "source_image": f"v{volume}.jpg",
                "image_sha256": "abc",
            }
        ],
    }


def test_load_source_character_records_preserves_duplicate_character_ids(tmp_path: Path) -> None:
    comics = tmp_path / "lore/canon/comics"
    write_character_file(
        comics / "characters/characters-volume-01.json",
        [character_item("steelhead", ["Steelhead"], 1)],
    )
    write_character_file(
        comics / "characters/characters-volume-02.json",
        [character_item("steelhead", ["Steelhead"], 2)],
    )

    records = load_source_character_records(comics)

    assert len(records) == 2
    assert {record.character_id for record in records} == {"steelhead"}
    assert records[0].record_id != records[1].record_id


def test_split_singletons_and_duplicates_keeps_only_hard_groups_for_model() -> None:
    records = [
        SourceCharacterRecord(record_id="a", character_id="solo", source_file="one.json", record_index=0),
        SourceCharacterRecord(record_id="b", character_id="steelhead", source_file="two.json", record_index=0),
        SourceCharacterRecord(record_id="c", character_id="steelhead", source_file="three.json", record_index=0),
    ]

    singletons, duplicates = split_singletons_and_duplicates(records)

    assert [peer.peer_card_id for peer in singletons] == ["solo"]
    assert [record.record_id for record in duplicates] == ["b", "c"]


def test_identity_prompt_prioritizes_honcho_card_coherence(tmp_path: Path) -> None:
    prompt = identity_map_prompt(
        input_path=tmp_path / "input.json",
        previous_output=None,
        previous_errors=[],
    )

    assert "Honcho-card coherence wins" in prompt
    assert "Steelhead as warrior heir" in prompt


def test_validate_identity_draft_requires_exact_record_coverage() -> None:
    records = [
        SourceCharacterRecord(record_id="a", character_id="x", source_file="one.json", record_index=0),
        SourceCharacterRecord(record_id="b", character_id="x", source_file="two.json", record_index=0),
    ]
    draft = IdentityMapDraft(
        peers=[
            PeerIdentity(
                peer_card_id="x",
                display_name="X",
                source_record_ids=["a", "a"],
                arc_label="merged",
                merge_policy="merged_same_persona",
                rationale="same persona",
                honcho_notes="base card",
            )
        ]
    )

    with pytest.raises(IdentityValidationError) as exc:
        validate_identity_draft(draft, records)

    assert "duplicate source_record_ids" in str(exc.value)
    assert "missing source_record_ids" in str(exc.value)


def test_validate_identity_draft_accepts_phase_split() -> None:
    records = [
        SourceCharacterRecord(record_id="early", character_id="steelhead", source_file="one.json", record_index=0),
        SourceCharacterRecord(record_id="late", character_id="steelhead", source_file="two.json", record_index=0),
    ]
    draft = IdentityMapDraft(
        peers=[
            PeerIdentity(
                peer_card_id="steelhead__warrior",
                display_name="Steelhead",
                source_record_ids=["early"],
                arc_label="warrior era",
                merge_policy="split_phase",
                rationale="early combat persona",
                honcho_notes="separate card",
            ),
            PeerIdentity(
                peer_card_id="steelhead__late-sealed",
                display_name="Steelhead",
                source_record_ids=["late"],
                arc_label="late sealed era",
                merge_policy="split_phase",
                rationale="late ruined persona",
                honcho_notes="separate card",
            ),
        ]
    )

    validate_identity_draft(draft, records)


def test_validate_peer_card_outputs_rejects_missing_identity_map(tmp_path: Path) -> None:
    assert validate_peer_card_outputs(tmp_path) == [f"missing {tmp_path / 'identity-map.json'}"]
    assert identity_status(tmp_path)["identity_map"] == "missing"
