from __future__ import annotations

from bridge.models import StoryIndexDocument, StoryIndexIngestRequest
from bridge.story_index import ingest_story_index


class FakeQdrantClient:
    last: "FakeQdrantClient | None" = None

    def __init__(self, *args: object, **kwargs: object) -> None:
        self.created: list[str] = []
        self.recreated: list[str] = []
        self.upserts: list[tuple[str, list[object]]] = []
        FakeQdrantClient.last = self

    def collection_exists(self, collection_name: str) -> bool:
        return False

    def create_collection(self, collection_name: str, vectors_config: object) -> None:
        self.created.append(collection_name)

    def recreate_collection(self, collection_name: str, vectors_config: object) -> None:
        self.recreated.append(collection_name)

    def upsert(self, collection_name: str, points: list[object]) -> None:
        self.upserts.append((collection_name, points))


def test_story_index_ingest_uses_requested_collection(monkeypatch) -> None:
    import qdrant_client

    monkeypatch.setattr(qdrant_client, "QdrantClient", FakeQdrantClient)
    req = StoryIndexIngestRequest(
        collection="sans_nom_story_test",
        replace=True,
        fake_embeddings=True,
        documents=[
            StoryIndexDocument(
                doc_id="story_bible:test",
                point_id="00000000-0000-0000-0000-000000000000",
                kind="story_bible",
                source_path="bible.json",
                source_sha256="source",
                text_sha256="text",
                text="Story bible with a law.",
                story_id="test",
            )
        ],
    )

    resp = ingest_story_index(req)

    assert resp.collection == "sans_nom_story_test"
    assert resp.count == 1
    assert resp.replaced is True
    assert FakeQdrantClient.last is not None
    assert FakeQdrantClient.last.recreated == ["sans_nom_story_test"]
    assert FakeQdrantClient.last.upserts[0][0] == "sans_nom_story_test"
