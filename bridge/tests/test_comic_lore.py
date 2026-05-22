from __future__ import annotations

import json
from pathlib import Path

from bridge.comic_lore import (
    extract_claude_json,
    ManifestEntry,
    normalize_character_item,
    PageRef,
    PageSummary,
    Paths,
    build_manifest,
    lore_documents,
    select_entries,
    strict_json_schema,
    validate_outputs,
    write_json_atomic,
)


def test_build_manifest_parses_volume_and_spread(tmp_path: Path) -> None:
    img_dir = tmp_path / "external/comics/meta-barons"
    img_dir.mkdir(parents=True)
    (img_dir / "001_The_Metabarons_1_p01.jpg").write_bytes(b"one")
    (img_dir / "071_The_Metabarons_3_p14-p15.jpg").write_bytes(b"spread")

    manifest = build_manifest(img_dir)

    assert manifest.page_count == 2
    assert manifest.entries[0].volume == 1
    assert manifest.entries[1].is_spread is True


def test_select_entries_limits_by_volume() -> None:
    entries = [
        ManifestEntry(
            page_id="a",
            ordinal=1,
            volume=1,
            page_label="p01",
            is_spread=False,
            source_image="a.jpg",
            image_sha256="x",
            byte_size=1,
        ),
        ManifestEntry(
            page_id="b",
            ordinal=2,
            volume=2,
            page_label="p01",
            is_spread=False,
            source_image="b.jpg",
            image_sha256="y",
            byte_size=1,
        ),
    ]

    assert [entry.page_id for entry in select_entries(entries, volume=2)] == ["b"]


def test_lore_documents_from_page(tmp_path: Path) -> None:
    paths = Paths(root=tmp_path, output_dir=tmp_path / "lore/packs/metabarons/comics")
    (tmp_path / "lore/packs/metabarons/LORE.md").parent.mkdir(parents=True, exist_ok=True)
    (tmp_path / "lore/packs/metabarons/LORE.md").write_text("# Master Lore\n\nFixture summary.")
    page = PageSummary(
        page_id="001_The_Metabarons_1_p01",
        volume=1,
        page_label="p01",
        is_spread=False,
        source_image="external/comics/meta-barons/001_The_Metabarons_1_p01.jpg",
        image_sha256="abc",
        confidence="high",
        visual_summary="A city-shaft surrounds the Metabunker.",
        plot_events=["Tonto is asked for another story."],
        characters_present=["Tonto"],
        locations=["Metabunker"],
        source_refs=[],
    )
    write_json_atomic(paths.pages / f"{page.page_id}.json", page.model_dump())

    docs = lore_documents(paths)

    page_docs = [doc for doc in docs if doc["kind"] == "comic_page"]
    assert len(page_docs) == 1
    assert "Tonto" in page_docs[0]["text"]
    assert any(doc["kind"] == "master_lore" for doc in docs)


def test_validate_outputs_accepts_character_array(tmp_path: Path) -> None:
    paths = Paths(root=tmp_path, output_dir=tmp_path / "lore/packs/metabarons/comics")
    char_dir = paths.characters
    char_dir.mkdir(parents=True)
    (char_dir / "characters.json").write_text(
        json.dumps([
            {
                "character_id": "tonto",
                "aliases": ["Tonto"],
                "source_refs": [],
            }
        ])
    )

    validate_outputs(paths, strict=True)


def test_strict_json_schema_sets_nested_additional_properties() -> None:
    schema = strict_json_schema(PageSummary.model_json_schema())

    assert schema["additionalProperties"] is False
    assert schema["$defs"]["PageRef"]["additionalProperties"] is False
    assert set(schema["required"]) == set(schema["properties"].keys())


def test_extract_claude_json_uses_structured_output() -> None:
    raw = json.dumps({
        "type": "result",
        "is_error": False,
        "result": "",
        "structured_output": {"ok": True},
    })

    assert json.loads(extract_claude_json(raw)) == {"ok": True}


def test_extract_claude_json_recovers_embedded_result_json() -> None:
    raw = json.dumps({
        "type": "result",
        "is_error": False,
        "result": "Done.\n{\"characters\": []}",
        "structured_output": None,
    })

    assert json.loads(extract_claude_json(raw)) == {"characters": []}


def test_normalize_character_item_accepts_common_claude_variants() -> None:
    page_ref = PageRef(page_id="p1", source_image="p1.jpg", image_sha256="abc")
    normalized = normalize_character_item(
        {
            "id": "tonto",
            "name": "Tonto",
            "appearances": [{"page_id": "p1", "note": "off-panel storyteller"}],
            "relationships": [{"character": "Lothar", "relation": "listener"}],
            "voice_notes": "reluctant storyteller",
            "source_refs": [{"page_id": "p1"}],
        },
        {"p1": page_ref},
    )

    assert normalized["character_id"] == "tonto"
    assert normalized["aliases"] == ["Tonto"]
    assert normalized["voice_notes"] == ["reluctant storyteller"]
    assert normalized["source_refs"] == [page_ref.model_dump()]
