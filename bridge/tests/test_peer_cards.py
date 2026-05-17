from __future__ import annotations

import json
from pathlib import Path

import pytest

from bridge.peer_cards import (
    CardDraft,
    DEFAULT_CARD_WRITER_EFFORT,
    DEFAULT_CARD_WRITER_MODEL,
    DEFAULT_CODEX_OAUTH_WRITER_EFFORT,
    DEFAULT_CODEX_OAUTH_WRITER_MODEL,
    IdentityMapDraft,
    IdentityValidationError,
    NonPeerSourceRecord,
    PeerIdentity,
    PeerMaterialReview,
    SourceCharacterRecord,
    VerifierReview,
    apply_deterministic_peer_overrides,
    build_peer_input,
    generate_peer_card,
    identity_map_prompt,
    identity_status,
    lothar_acceptance_rubric,
    load_source_character_records,
    non_peer_artifact_path,
    peer_card_artifact_path,
    peer_iterations_dir,
    resolve_codex_oauth_service_tier,
    resolve_writer_model_effort,
    peer_material_prompt,
    run_writer,
    split_singletons_and_duplicates,
    validate_card_contract,
    validate_identity_draft,
    validate_lothar_card_rubric,
    validate_peer_card_outputs,
    verifier_prompt,
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
    comics = tmp_path / "lore/packs/metabarons/comics"
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
    assert records[0].source_refs == [
        {"page_id": "v1p1", "source_image": "v1.jpg", "image_sha256": "abc"}
    ]


def test_split_singletons_and_duplicates_keeps_only_hard_groups_for_model() -> None:
    records = [
        SourceCharacterRecord(record_id="a", character_id="solo", source_file="one.json", record_index=0),
        SourceCharacterRecord(record_id="b", character_id="steelhead", source_file="two.json", record_index=0),
        SourceCharacterRecord(record_id="c", character_id="steelhead", source_file="three.json", record_index=0),
        SourceCharacterRecord(record_id="d", character_id="lady_edna", aliases=["Edna"], source_file="four.json", record_index=0),
        SourceCharacterRecord(record_id="e", character_id="edna", aliases=["Edna of Castaka"], source_file="five.json", record_index=0),
        SourceCharacterRecord(record_id="f", character_id="iku-itta", aliases=["Iku-Itta"], source_file="six.json", record_index=0),
        SourceCharacterRecord(record_id="g", character_id="iko-itta", aliases=["Iko-Itta"], source_file="seven.json", record_index=0),
        SourceCharacterRecord(record_id="h", character_id="ik-uta", aliases=["Ik'uta"], source_file="eight.json", record_index=0),
    ]

    singletons, duplicates = split_singletons_and_duplicates(records)

    assert [peer.peer_card_id for peer in singletons] == ["solo"]
    assert [record.record_id for record in duplicates] == ["b", "c", "d", "e", "f", "g", "h"]


def test_deterministic_peer_overrides_merge_shabda_oud_group_roles() -> None:
    records = [
        SourceCharacterRecord(record_id="ms1", character_id="mother-superior", source_file="one.json", record_index=0),
        SourceCharacterRecord(record_id="ms2", character_id="mother-superior-of-the-shabda-oud", source_file="two.json", record_index=0),
        SourceCharacterRecord(record_id="w1", character_id="the-three-whore-priestesses", source_file="three.json", record_index=0),
        SourceCharacterRecord(record_id="w2", character_id="the-shabda-oud", source_file="four.json", record_index=0),
        SourceCharacterRecord(record_id="w3", character_id="shabda_oud", source_file="five.json", record_index=0),
        SourceCharacterRecord(record_id="w4", character_id="the-four-whore-priestesses-of-shabda-oud", source_file="six.json", record_index=0),
        SourceCharacterRecord(record_id="solo", character_id="solo", source_file="seven.json", record_index=0),
    ]
    draft = IdentityMapDraft(
        peers=[
            PeerIdentity(
                peer_card_id=record.character_id,
                display_name=record.character_id,
                source_record_ids=[record.record_id],
                arc_label="fixture",
                merge_policy="single_record",
                rationale="fixture",
                honcho_notes="fixture",
            )
            for record in records
        ]
    )

    merged = apply_deterministic_peer_overrides(draft, records)

    peer_by_id = {peer.peer_card_id: peer for peer in merged.peers}
    assert "mother-superior" not in peer_by_id
    assert "mother-superior-of-the-shabda-oud" not in peer_by_id
    assert "the-three-whore-priestesses" not in peer_by_id
    assert "the-four-whore-priestesses-of-shabda-oud" not in peer_by_id
    assert "the-shabda-oud" not in peer_by_id
    assert peer_by_id["shabda-oud-mother-superiors"].source_record_ids == ["ms1", "ms2"]
    assert peer_by_id["shabda-oud-witches"].source_record_ids == ["w1", "w2", "w3", "w4"]
    assert peer_by_id["shabda-oud-witches"].merge_policy == "merged_group_role"
    assert peer_by_id["solo"].source_record_ids == ["solo"]
    validate_identity_draft(merged, records)


def test_identity_prompt_prioritizes_honcho_card_coherence(tmp_path: Path) -> None:
    prompt = identity_map_prompt(
        input_path=tmp_path / "input.json",
        previous_output=None,
        previous_errors=[],
    )

    assert "Honcho-card coherence wins" in prompt
    assert "Steelhead as warrior heir" in prompt


def test_writer_prompt_reads_wiki_and_link_matrix_before_deep_dataset(tmp_path: Path) -> None:
    from bridge.peer_cards import writer_prompt

    prompt = writer_prompt(input_path=tmp_path / "input.json", previous_review_path=None)

    assert "wiki_lore.path" in prompt
    assert "link_matrix.path" in prompt
    assert "peer_card_encoding_deliberation.path" in prompt
    assert "Use the link matrix as source support" in prompt
    assert "internal organizing sentence" in prompt
    assert "refuses to become" in prompt
    assert "wound-shape" in prompt
    assert "eleven words" in prompt
    assert "Leave out verdicts" in prompt
    assert "The target is exactly the `peer_identity`" in prompt
    assert "do not inflate it into a full-arc biography" in prompt
    assert "council_relationship_matrix" not in prompt


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


def test_validate_identity_draft_treats_non_peer_records_as_evidence_not_peers() -> None:
    records = [
        SourceCharacterRecord(record_id="iku", character_id="iku-itta", source_file="one.json", record_index=0),
        SourceCharacterRecord(
            record_id="daughters",
            character_id="iku-tta-daughters",
            source_file="two.json",
            record_index=0,
            peer_candidate=False,
            non_peer_reason="plural fact bundle",
            related_peer_ids=["iku-itta"],
        ),
    ]
    non_peer_records = [
        NonPeerSourceRecord(
            source_record_id="daughters",
            reason="plural fact bundle",
            related_peer_ids=["iku-itta"],
        )
    ]
    draft = IdentityMapDraft(
        peers=[
            PeerIdentity(
                peer_card_id="iku-itta",
                display_name="Iku-Tta",
                source_record_ids=["iku"],
                arc_label="single servant identity",
                merge_policy="single_record",
                rationale="one character",
                honcho_notes="base card",
            )
        ]
    )

    validate_identity_draft(draft, records, non_peer_records)

    draft.peers[0].source_record_ids.append("daughters")
    with pytest.raises(IdentityValidationError) as exc:
        validate_identity_draft(draft, records, non_peer_records)

    assert "references non-peer source record daughters" in str(exc.value)


def test_validate_identity_draft_rejects_separator_only_peer_id_duplicates() -> None:
    records = [
        SourceCharacterRecord(record_id="a", character_id="othon-von-salza", source_file="one.json", record_index=0),
        SourceCharacterRecord(record_id="b", character_id="othon_von_salza", source_file="two.json", record_index=0),
    ]
    draft = IdentityMapDraft(
        peers=[
            PeerIdentity(
                peer_card_id="othon-von-salza",
                display_name="Othon von Salza",
                source_record_ids=["a"],
                arc_label="fixture",
                merge_policy="single_record",
                rationale="fixture",
                honcho_notes="fixture",
            ),
            PeerIdentity(
                peer_card_id="othon_von_salza",
                display_name="Othon von Salza",
                source_record_ids=["b"],
                arc_label="fixture",
                merge_policy="single_record",
                rationale="fixture",
                honcho_notes="fixture",
            ),
        ]
    )

    with pytest.raises(IdentityValidationError) as exc:
        validate_identity_draft(draft, records)

    assert "differ only by '-'/'_' separators" in str(exc.value)


def test_validate_peer_card_outputs_rejects_missing_identity_map(tmp_path: Path) -> None:
    assert validate_peer_card_outputs(tmp_path) == [f"missing {tmp_path / 'identity-map.json'}"]
    assert identity_status(tmp_path)["identity_map"] == "missing"


def test_card_contract_rejects_bad_prefix_sources_and_duplicates() -> None:
    errors = validate_card_contract(
        [
            "Name: Lothar",
            "FACT: invalid prefix",
            "TRAIT: See lore/packs/metabarons/comics/pages/x.json",
            "Name: Lothar",
        ]
    )

    assert any("unsupported prefix" in error for error in errors)
    assert any("source refs" in error for error in errors)
    assert any("duplicate fact" in error for error in errors)


def test_verifier_prompt_accepts_source_thin_manual_review_cards(tmp_path: Path) -> None:
    prompt = verifier_prompt(
        input_path=tmp_path / "input.json",
        draft_path=tmp_path / "draft.json",
        local_errors=[],
    )

    assert "source-thin minor characters" in prompt
    assert "do not push the Writer to invent lore" in prompt
    assert "manual_review_notes" in prompt
    assert "internal organizing sentence" in prompt
    assert "refuse to become" in prompt
    assert "exact transformative lines" in prompt
    assert "final verdicts" in prompt


def test_lothar_rubric_rejects_flat_comic_relief_card() -> None:
    card = [
        "Name: Lothar",
        "ROLE: small comic relief companion to Tonto.",
        "TRAIT: listens passively while Tonto tells stories.",
    ]

    errors = validate_lothar_card_rubric(card)

    assert any("Master Lothar" in error for error in errors)
    assert any("procedural/operator" in error for error in errors)
    assert any("small comic relief" in error for error in errors)


def test_lothar_rubric_accepts_required_axes() -> None:
    card = [
        "Name: Lothar",
        "ALIAS: Master Lothar.",
        "ROLE: technical operator who handles timing, bio-electrogram procedure, metacraft readiness, holographic force, and weapons.",
        "RELATIONSHIP: tonto (Tonto): coerces Tonto, tears and dismembers him, demands 'sir,' and enforces narration when status reverses.",
        "TRAIT: anti-romantic machine logic treats love as human foolishness.",
        "TRAIT: curiosity drives him into dangerous contact with human motives and impossible stories.",
        "TRAIT: loss of faith and despair can weaken his projected force.",
        "TRAIT: feels lust, apology, hatred, and pain despite machine discipline.",
        "TRAIT: Lothar is the amnesiac Steelhead hidden inside armor after Nameless erased his memory and made him believe he was a robot.",
        "VOICE: precise, status-conscious, procedural, impatient, and capable of sudden emotional nakedness.",
        "BOUNDARY: Do not reveal Lothar's Steelhead identity before the story earns the Tonto/Nameless reveal.",
        "BOUNDARY: Lothar may be comic or absurd, but not a joke device that deflates every tense scene.",
    ]

    assert validate_lothar_card_rubric(card) == []


def test_lothar_rubric_rejects_known_misattributed_repair_claims() -> None:
    card = [
        "Name: Lothar",
        "ALIAS: Master Lothar.",
        "ROLE: technical operator who handles timing, bio-electrogram procedure, metacraft readiness, holographic force, and weapons.",
        "RELATIONSHIP: tonto (Tonto): companion, narrator, repairer, dismembered target, and target of Lothar's coercion.",
        "RELATIONSHIP: tonto (Tonto): Lothar demands 'sir' and enforces narration when status reverses.",
        "TRAIT: anti-romantic machine logic treats love as human foolishness.",
        "TRAIT: curiosity drives him into dangerous contact with human motives and impossible stories.",
        "TRAIT: loss of faith and despair can weaken his projected force.",
        "TRAIT: feels lust, apology, hatred, and pain despite machine discipline.",
        "TRAIT: Lothar is the amnesiac Steelhead hidden inside armor after Nameless erased his memory and made him believe he was a robot.",
        "VOICE: precise, status-conscious, procedural, impatient, and capable of sudden emotional nakedness.",
        "INSTRUCTION: Lothar may threaten circuit repair to obtain compliance.",
        "BOUNDARY: Do not reveal Lothar's Steelhead identity before the story earns the Tonto/Nameless reveal.",
        "BOUNDARY: Lothar may be comic or absurd, but not a joke device that deflates every tense scene.",
    ]

    errors = validate_lothar_card_rubric(card)

    assert any("repairer" in error for error in errors)
    assert any("circuit-repair" in error for error in errors)


def test_lothar_rubric_mentions_identity_preservation() -> None:
    rubric = lothar_acceptance_rubric()

    assert any("Lothar stays one peer card" in item for item in rubric)
    assert any("Lothar is in fact Steelhead" in item for item in rubric)


def test_build_peer_input_lothar_from_fixture_includes_related_ids(tmp_path: Path) -> None:
    root = tmp_path
    comics_dir = root / "lore/packs/metabarons/comics"
    peer_cards_dir = comics_dir / "peer-cards"
    write_character_file(
        comics_dir / "characters/characters-volume-01.json",
        [character_item("lothar", ["Lothar"], 1)],
    )
    write_character_file(
        comics_dir / "characters/characters-volume-02.json",
        [character_item("lothar", ["Master Lothar"], 2)],
    )

    source_records = load_source_character_records(comics_dir)
    identity = {
        "version": 1,
        "input_record_count": 2,
        "raw_character_id_count": 1,
        "peer_count": 5,
        "model": {
            "writer_model": "test",
            "writer_effort": "low",
            "iterations": 1,
            "generated_at": "2026-05-04T00:00:00+00:00",
        },
        "source_records": [record.model_dump() for record in source_records],
        "peers": [
            {
                "peer_card_id": "lothar",
                "display_name": "Lothar",
                "source_record_ids": [record.record_id for record in source_records],
                "arc_label": "single tense identity",
                "merge_policy": "merged_same_persona",
                "rationale": "phase tension belongs in one card",
                "honcho_notes": "encode contradiction",
            },
            peer_fixture("tonto"),
            peer_fixture("the-metabaron"),
            peer_fixture("aghora"),
            peer_fixture("steelhead__late-sealed"),
        ],
    }
    peer_cards_dir.mkdir(parents=True)
    (peer_cards_dir / "identity-map.json").write_text(json.dumps(identity))

    peer_input = build_peer_input(root, comics_dir, peer_cards_dir, "lothar")

    assert peer_input.identity_map_path.endswith("identity-map.json")
    assert len(peer_input.source_records) == 2
    assert peer_input.related_peer_ids == ["tonto", "the-metabaron", "aghora", "steelhead__late-sealed"]
    assert "wiki_lore" in peer_input.context_entrypoints
    assert "link_matrix" in peer_input.context_entrypoints
    assert "peer_card_encoding_deliberation" in peer_input.context_entrypoints
    assert "council_relationship_matrix" not in peer_input.context_entrypoints
    assert "lothar_cast_doc" in peer_input.context_entrypoints
    assert peer_input.lothar_acceptance_rubric


def test_generate_peer_card_resumes_after_latest_review(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    root = tmp_path
    comics_dir = root / "lore/packs/metabarons/comics"
    peer_cards_dir = comics_dir / "peer-cards"
    write_character_file(
        comics_dir / "characters/characters-volume-01.json",
        [character_item("lothar", ["Lothar"], 1)],
    )
    source_records = load_source_character_records(comics_dir)
    identity = {
        "version": 1,
        "input_record_count": 1,
        "raw_character_id_count": 1,
        "peer_count": 1,
        "model": {
            "writer_model": "test",
            "writer_effort": "low",
            "iterations": 1,
            "generated_at": "2026-05-04T00:00:00+00:00",
        },
        "source_records": [record.model_dump() for record in source_records],
        "peers": [
            {
                "peer_card_id": "lothar",
                "display_name": "Lothar",
                "source_record_ids": [source_records[0].record_id],
                "arc_label": "single",
                "merge_policy": "single_record",
                "rationale": "fixture",
                "honcho_notes": "fixture",
            }
        ],
    }
    peer_cards_dir.mkdir(parents=True)
    (peer_cards_dir / "identity-map.json").write_text(json.dumps(identity))
    iterations_dir = peer_iterations_dir(peer_cards_dir, "lothar")
    iterations_dir.mkdir(parents=True)
    review_path = iterations_dir / "01-review.json"
    review_path.write_text(
        VerifierReview(
            accepted=False,
            revision_instructions=["revise from prior review"],
        ).model_dump_json()
    )

    seen_previous_review: list[Path | None] = []

    def fake_writer(**kwargs: object) -> CardDraft:
        seen_previous_review.append(kwargs["previous_review_path"])  # type: ignore[index]
        return CardDraft(
            peer_card_id="lothar",
            display_name="Lothar",
            card=[
                "Name: Lothar",
                "ALIAS: Master Lothar.",
                "ROLE: technical operator who handles timing, bio-electrogram procedure, metacraft readiness, holographic force, and weapons.",
                "RELATIONSHIP: tonto (Tonto): coerces Tonto, tears and dismembers him, demands 'sir,' and enforces narration when status reverses.",
                "TRAIT: anti-romantic machine logic treats love as human foolishness.",
                "TRAIT: curiosity drives him into dangerous contact with human motives and impossible stories.",
                "TRAIT: loss of faith and despair can weaken his projected force.",
                "TRAIT: feels lust, apology, hatred, and pain despite machine discipline.",
                "TRAIT: Lothar is the amnesiac Steelhead hidden inside armor after Nameless erased his memory and made him believe he was a robot.",
                "VOICE: precise, status-conscious, procedural, impatient, and capable of sudden emotional nakedness.",
                "BOUNDARY: Do not reveal Lothar's Steelhead identity before the story earns the Tonto/Nameless reveal.",
                "BOUNDARY: Lothar may be comic or absurd, but not a joke device that deflates every tense scene.",
            ],
        )

    def fake_verifier(**kwargs: object) -> VerifierReview:
        return VerifierReview(accepted=True)

    monkeypatch.setattr("bridge.peer_cards.run_writer", fake_writer)
    monkeypatch.setattr("bridge.peer_cards.run_verifier", fake_verifier)

    artifact = generate_peer_card(
        root=root,
        comics_dir=comics_dir,
        peer_cards_dir=peer_cards_dir,
        peer_id="lothar",
        force=False,
        max_iterations=5,
        writer_model="writer",
        writer_effort="max",
        verifier_model="verifier",
        verifier_effort="xhigh",
    )

    assert seen_previous_review == [review_path]
    assert artifact.model.iterations == 2
    assert peer_card_artifact_path(peer_cards_dir, "lothar").exists()


def test_generate_peer_card_preserves_orphan_draft_on_resume(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    root = tmp_path
    comics_dir = root / "lore/packs/metabarons/comics"
    peer_cards_dir = comics_dir / "peer-cards"
    write_character_file(
        comics_dir / "characters/characters-volume-01.json",
        [character_item("solo", ["Solo"], 1)],
    )
    source_records = load_source_character_records(comics_dir)
    identity = {
        "version": 1,
        "input_record_count": 1,
        "raw_character_id_count": 1,
        "peer_count": 1,
        "model": {
            "writer_model": "test",
            "writer_effort": "low",
            "iterations": 1,
            "generated_at": "2026-05-04T00:00:00+00:00",
        },
        "source_records": [record.model_dump() for record in source_records],
        "peers": [
            {
                "peer_card_id": "solo",
                "display_name": "Solo",
                "source_record_ids": [source_records[0].record_id],
                "arc_label": "single",
                "merge_policy": "single_record",
                "rationale": "fixture",
                "honcho_notes": "fixture",
            }
        ],
    }
    peer_cards_dir.mkdir(parents=True)
    (peer_cards_dir / "identity-map.json").write_text(json.dumps(identity))
    iterations_dir = peer_iterations_dir(peer_cards_dir, "solo")
    iterations_dir.mkdir(parents=True)
    orphan_path = iterations_dir / "01-draft.json"
    orphan_path.write_text('{"peer_card_id":"solo","display_name":"Solo","card":["Name: orphan"]}')

    seen_paths: list[Path] = []

    def fake_writer(**kwargs: object) -> CardDraft:
        return CardDraft(peer_card_id="solo", display_name="Solo", card=["Name: Solo"])

    def fake_verifier(**kwargs: object) -> VerifierReview:
        seen_paths.append(kwargs["draft_path"])  # type: ignore[index]
        return VerifierReview(accepted=True)

    monkeypatch.setattr("bridge.peer_cards.run_writer", fake_writer)
    monkeypatch.setattr("bridge.peer_cards.run_verifier", fake_verifier)

    artifact = generate_peer_card(
        root=root,
        comics_dir=comics_dir,
        peer_cards_dir=peer_cards_dir,
        peer_id="solo",
        force=False,
        max_iterations=5,
        writer_model="writer",
        writer_effort="max",
        verifier_model="verifier",
        verifier_effort="high",
    )

    assert seen_paths == [iterations_dir / "02-draft.json"]
    assert orphan_path.read_text() == '{"peer_card_id":"solo","display_name":"Solo","card":["Name: orphan"]}'
    assert artifact.model.iterations == 2


def test_generate_peer_card_preflight_marks_non_peer_lore(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    root = tmp_path
    comics_dir = root / "lore/packs/metabarons/comics"
    peer_cards_dir = comics_dir / "peer-cards"
    write_character_file(
        comics_dir / "characters/characters-volume-07.json",
        [
            character_item("the-invincible-warrior", ["The Invincible Warrior"], 7),
            character_item("tonto", ["Tonto"], 7),
            character_item("lothar", ["Lothar"], 7),
        ],
    )
    source_records = load_source_character_records(comics_dir)
    record_by_character_id = {record.character_id: record for record in source_records}
    identity = {
        "version": 1,
        "input_record_count": 1,
        "raw_character_id_count": 1,
        "peer_count": 3,
        "model": {
            "writer_model": "test",
            "writer_effort": "low",
            "iterations": 1,
            "generated_at": "2026-05-04T00:00:00+00:00",
        },
        "source_records": [record.model_dump() for record in source_records],
        "peers": [
            {
                "peer_card_id": "the-invincible-warrior",
                "display_name": "The Invincible Warrior",
                "source_record_ids": [source_records[0].record_id],
                "arc_label": "scar mystery hook",
                "merge_policy": "single_record",
                "rationale": "fixture",
                "honcho_notes": "fixture",
            },
            {
                "peer_card_id": "tonto",
                "display_name": "Tonto",
                "source_record_ids": [record_by_character_id["tonto"].record_id],
                "arc_label": "fixture",
                "merge_policy": "single_record",
                "rationale": "fixture related peer",
                "honcho_notes": "fixture only",
            },
            {
                "peer_card_id": "lothar",
                "display_name": "Lothar",
                "source_record_ids": [record_by_character_id["lothar"].record_id],
                "arc_label": "fixture",
                "merge_policy": "single_record",
                "rationale": "fixture related peer",
                "honcho_notes": "fixture only",
            },
        ],
    }
    peer_cards_dir.mkdir(parents=True)
    (peer_cards_dir / "identity-map.json").write_text(json.dumps(identity))

    def fake_material_verifier(**kwargs: object) -> PeerMaterialReview:
        return PeerMaterialReview(
            peer_card_id="the-invincible-warrior",
            peer_card_material=False,
            should_mark_non_peer_lore=True,
            non_peer_reason="Title/mystery hook, not a distinct peer persona.",
            related_peer_ids=["tonto", "lothar", "missing-peer"],
        )

    def fail_writer(**kwargs: object) -> CardDraft:
        raise AssertionError("writer should not run for non-peer lore")

    monkeypatch.setattr("bridge.peer_cards.run_peer_material_verifier", fake_material_verifier)
    monkeypatch.setattr("bridge.peer_cards.run_writer", fail_writer)

    artifact = generate_peer_card(
        root=root,
        comics_dir=comics_dir,
        peer_cards_dir=peer_cards_dir,
        peer_id="the-invincible-warrior",
        force=False,
        max_iterations=5,
        writer_model="writer",
        writer_effort="max",
        verifier_model="verifier",
        verifier_effort="high",
        material_preflight=True,
    )

    assert artifact.non_peer_reason == "Title/mystery hook, not a distinct peer persona."
    assert artifact.related_peer_ids == ["tonto", "lothar"]
    assert non_peer_artifact_path(peer_cards_dir, "the-invincible-warrior").exists()
    updated_identity = json.loads((peer_cards_dir / "identity-map.json").read_text())
    assert updated_identity["peer_count"] == 2
    assert "the-invincible-warrior" not in {peer["peer_card_id"] for peer in updated_identity["peers"]}
    assert updated_identity["source_records"][0]["peer_candidate"] is False
    assert updated_identity["non_peer_source_records"][0]["source_record_id"] == source_records[0].record_id

    updated_characters = json.loads((comics_dir / "characters/characters-volume-07.json").read_text())
    assert updated_characters[0]["peer_candidate"] is False
    assert updated_characters[0]["related_peer_ids"] == ["tonto", "lothar"]


def test_peer_material_prompt_marks_lore_only_records_before_writer() -> None:
    prompt = peer_material_prompt(Path("/tmp/input.json"))

    assert "before the expensive Writer loop runs" in prompt
    assert "not peer-card material" in prompt
    assert "title/mystery-hook records" in prompt


def test_run_writer_uses_codex_oauth_backend(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    input_path = tmp_path / "input.json"
    input_path.write_text("{}")
    seen: dict[str, object] = {}

    def fake_codex_oauth_json(**kwargs: object) -> str:
        seen.update(kwargs)
        return CardDraft(peer_card_id="solo", display_name="Solo", card=["Name: Solo"]).model_dump_json()

    monkeypatch.setattr("bridge.peer_cards.run_codex_oauth_json", fake_codex_oauth_json)

    draft = run_writer(
        root=tmp_path,
        input_path=input_path,
        previous_review_path=None,
        backend="codex-oauth",
        model="gpt-5.5",
        effort="medium",
    )

    assert draft.peer_card_id == "solo"
    assert seen["root"] == tmp_path
    assert seen["schema_or_model"] is CardDraft
    assert seen["model"] == "gpt-5.5"
    assert seen["effort"] == "medium"
    assert seen["service_tier"] == ""


def test_codex_oauth_writer_defaults_to_gpt55_medium(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("SANS_NOM_PEER_CARD_WRITER_MODEL", raising=False)
    monkeypatch.delenv("SANS_NOM_PEER_CARD_WRITER_EFFORT", raising=False)

    model, effort = resolve_writer_model_effort(
        "codex-oauth",
        DEFAULT_CARD_WRITER_MODEL,
        DEFAULT_CARD_WRITER_EFFORT,
    )

    assert model == DEFAULT_CODEX_OAUTH_WRITER_MODEL
    assert effort == DEFAULT_CODEX_OAUTH_WRITER_EFFORT


def test_codex_oauth_service_tier_maps_fast_alias_to_priority(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("SANS_NOM_CODEX_OAUTH_SERVICE_TIER", "fast")

    assert resolve_codex_oauth_service_tier("") == "priority"
    assert resolve_codex_oauth_service_tier("priority") == "priority"


def peer_fixture(peer_card_id: str) -> dict[str, object]:
    return {
        "peer_card_id": peer_card_id,
        "display_name": peer_card_id.replace("-", " ").replace("_", " ").title(),
        "source_record_ids": [],
        "arc_label": "fixture",
        "merge_policy": "single_record",
        "rationale": "fixture related peer",
        "honcho_notes": "fixture only",
    }
