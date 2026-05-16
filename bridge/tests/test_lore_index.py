from __future__ import annotations

from pathlib import Path

from bridge.comic_lore import PageSummary, VolumeSummary, write_json_atomic
from bridge.lore_index import (
    DEFAULT_COMICS_DIR,
    build_documents,
    ensure_unique_documents,
    fake_embedding,
    point_id_for_doc_id,
    split_markdown,
    validate_index,
    write_index,
)


def test_build_documents_includes_all_lore_families(tmp_path: Path) -> None:
    make_minimal_corpus(tmp_path)

    docs = build_documents(tmp_path)
    kinds = {doc.kind for doc in docs}

    assert {
        "comic_page",
        "comic_volume",
        "character_record",
        "peer_card",
        "non_peer_lore",
        "identity_map_entry",
        "wiki_lore",
        "link_matrix",
    } <= kinds
    assert any(doc.doc_id == "peer_card:lothar" for doc in docs)
    assert any(doc.doc_id == "non_peer_lore:bushitaka" for doc in docs)
    assert any(doc.doc_id == "identity_map:peer:lothar" for doc in docs)


def test_character_records_stay_per_volume(tmp_path: Path) -> None:
    make_minimal_corpus(tmp_path)
    characters = tmp_path / DEFAULT_COMICS_DIR / "characters"
    write_json_atomic(characters / "characters-volume-02.json", [{
        "character_id": "lothar",
        "aliases": ["Lothar late"],
        "source_refs": [],
    }])

    docs = [doc for doc in build_documents(tmp_path) if doc.kind == "character_record" and doc.character_id == "lothar"]

    assert [doc.doc_id for doc in docs] == [
        "character_record:characters-volume-01.json#001:lothar",
        "character_record:characters-volume-02.json#001:lothar",
    ]


def test_write_and_validate_index_detects_source_drift(tmp_path: Path) -> None:
    make_minimal_corpus(tmp_path)
    index_dir = tmp_path / "lore/canon/index"
    docs = build_documents(tmp_path)

    write_index(tmp_path, index_dir, docs, collection="test_collection")

    assert validate_index(tmp_path, index_dir) == 0
    page_path = tmp_path / DEFAULT_COMICS_DIR / "pages/001_The_Metabarons_1_p01.json"
    page_path.write_text(page_path.read_text() + "\n")
    assert validate_index(tmp_path, index_dir) > 0


def test_point_id_is_uuidv5_stable() -> None:
    doc_id = "peer_card:lothar"

    assert point_id_for_doc_id(doc_id) == point_id_for_doc_id(doc_id)
    assert point_id_for_doc_id(doc_id) != point_id_for_doc_id("peer_card:tonto")


def test_duplicate_documents_are_rejected(tmp_path: Path) -> None:
    make_minimal_corpus(tmp_path)
    doc = build_documents(tmp_path)[0]

    try:
        ensure_unique_documents([doc, doc])
    except ValueError as exc:
        assert "duplicate doc_id" in str(exc)
    else:
        raise AssertionError("duplicate document was accepted")


def test_markdown_heading_split_and_long_fallback(tmp_path: Path) -> None:
    path = tmp_path / "matrix.md"
    path.write_text("# Root\n\n" + ("alpha beta\n\n" * 1000))

    chunks = split_markdown(path, "link_matrix")

    assert chunks
    assert all(len(chunk.text) <= 6000 for chunk in chunks)
    assert chunks[0].doc_id.startswith("link_matrix:root")


def test_fake_embedding_is_deterministic() -> None:
    first = fake_embedding("Lothar is Steelhead")
    second = fake_embedding("Lothar is Steelhead")

    assert first == second
    assert len(first) == 16


def make_minimal_corpus(root: Path) -> None:
    comics = root / DEFAULT_COMICS_DIR
    pages = comics / "pages"
    volumes = comics / "volumes"
    characters = comics / "characters"
    peer_cards = comics / "peer-cards"
    pages.mkdir(parents=True)
    volumes.mkdir(parents=True)
    characters.mkdir(parents=True)
    (peer_cards / "cards/accepted").mkdir(parents=True)
    (peer_cards / "cards/non-peer").mkdir(parents=True)
    (root / "lore/canon/wiki").mkdir(parents=True)
    (root / "lore/canon/links").mkdir(parents=True)

    page = PageSummary(
        page_id="001_The_Metabarons_1_p01",
        volume=1,
        page_label="p01",
        is_spread=False,
        source_image="external/comics/meta-barons/001_The_Metabarons_1_p01.jpg",
        image_sha256="image-hash",
        confidence="high",
        visual_summary="Tonto begins a story in the Metabunker city-shaft.",
        plot_events=["Lothar asks for a story."],
        characters_present=["Tonto", "Lothar"],
        locations=["Metabunker"],
        source_refs=[],
    )
    write_json_atomic(pages / f"{page.page_id}.json", page.model_dump())

    volume = VolumeSummary(
        volume=1,
        title="Volume One",
        page_ids=[page.page_id],
        source_images=[page.source_image],
        chronology=["Tonto narrates to Lothar."],
        major_events=["The frame story begins."],
    )
    write_json_atomic(volumes / "volume-01.json", volume.model_dump())

    write_json_atomic(characters / "characters-volume-01.json", [{
        "character_id": "lothar",
        "aliases": ["Lothar"],
        "relationships": ["character: tonto; relation: listener"],
        "source_refs": [],
    }])
    write_json_atomic(peer_cards / "cards/accepted/lothar.json", {
        "peer_card_id": "lothar",
        "display_name": "Lothar",
        "card": ["Name: Lothar", "TRAIT: Lothar is Steelhead but does not know it"],
        "source_record_ids": ["characters-volume-01.json#001:lothar"],
        "source_refs": [],
    })
    write_json_atomic(peer_cards / "cards/non-peer/bushitaka.json", {
        "peer_card_id": "bushitaka",
        "non_peer_reason": "Bushitaka is a law, not a peer.",
        "related_peer_ids": ["aghora"],
        "source_record_ids": ["characters-volume-16.json#007:bushitaka"],
        "verifier_report": {"manual_review_notes": ["Keep as non-peer lore."]},
    })
    write_json_atomic(peer_cards / "identity-map.json", {
        "peers": [{
            "peer_card_id": "lothar",
            "display_name": "Lothar",
            "arc_label": "single identity",
            "merge_policy": "merged_same_persona",
            "rationale": "Contradictions stay in one peer card.",
            "honcho_notes": "Do not split.",
            "source_record_ids": ["characters-volume-01.json#001:lothar"],
        }],
        "non_peer_source_records": [{
            "source_record_id": "characters-volume-16.json#007:bushitaka",
            "reason": "Doctrine only.",
            "related_peer_ids": ["aghora"],
        }],
    })
    (root / "lore/canon/wiki/lore.md").write_text("# Wiki Lore\n\nSans Nom ends the dynasty.")
    (root / "lore/canon/links/matrix.md").write_text("# Matrix\n\nLothar is Steelhead through hidden identity.")
