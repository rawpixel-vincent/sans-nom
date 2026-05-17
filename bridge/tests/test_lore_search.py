from __future__ import annotations

from fastapi.testclient import TestClient

from bridge import main
from bridge.lore_search import build_filter, clamp_k, snippet_from_hit
from bridge.models import LoreSearchRequest, LoreSearchResponse, LoreSnippet


class FakeLoreSearch:
    def __init__(self, response: LoreSearchResponse | None = None, error: Exception | None = None) -> None:
        self.response = response or LoreSearchResponse()
        self.error = error
        self.requests: list[LoreSearchRequest] = []

    def search(self, req: LoreSearchRequest) -> LoreSearchResponse:
        self.requests.append(req)
        if self.error:
            raise self.error
        return self.response


def test_clamp_k_bounds() -> None:
    assert clamp_k(0) == 1
    assert clamp_k(5) == 5
    assert clamp_k(50) == 20


def test_build_filter_includes_payload_fields() -> None:
    qdrant_filter = build_filter(LoreSearchRequest(
        query="x",
        kind=["peer_card"],
        peer_id=["lothar"],
        character_id=["steelhead"],
        volume=[13],
    ))

    dumped = qdrant_filter.model_dump()
    keys = [item["key"] for item in dumped["must"]]
    values = [item["match"]["value"] for item in dumped["must"]]
    assert keys == ["kind", "peer_id", "character_id", "volume"]
    assert values == ["peer_card", "lothar", "steelhead", 13]


def test_build_filter_uses_any_for_multiple_values() -> None:
    qdrant_filter = build_filter(LoreSearchRequest(query="x", kind=["peer_card", "comic_page"]))

    dumped = qdrant_filter.model_dump()
    assert dumped["must"][0]["key"] == "kind"
    assert dumped["must"][0]["match"]["any"] == ["peer_card", "comic_page"]


def test_snippet_preview_does_not_duplicate_full_text_payload() -> None:
    class Hit:
        id = "point"
        score = 0.5
        payload = {
            "doc_id": "comic_page:one",
            "text": "x" * 3000,
            "source_path": "lore/packs/metabarons/comics/pages/one.json",
            "kind": "comic_page",
        }

    snippet = snippet_from_hit(Hit())

    assert len(snippet.text) < 2000
    assert "text" not in snippet.payload


def test_lore_search_route_success(monkeypatch) -> None:
    fake = FakeLoreSearch(LoreSearchResponse(snippets=[
        LoreSnippet(
            doc_id="peer_card:lothar",
            text="Lothar is Steelhead.",
            source_path="lore/packs/metabarons/comics/peer-cards/cards/accepted/lothar.json",
            score=0.9,
            kind="peer_card",
            peer_id="lothar",
        )
    ]))
    monkeypatch.setattr(main, "lore_search", fake)
    monkeypatch.delenv("SIDECAR_TOKEN", raising=False)

    response = TestClient(main.app).post("/v1/lore/search", json={"collection": "story", "query": "Lothar is Steelhead", "k": 50})

    assert response.status_code == 200
    body = response.json()
    assert body["snippets"][0]["doc_id"] == "peer_card:lothar"
    assert fake.requests[0].k == 50
    assert fake.requests[0].collection == "story"


def test_lore_search_route_auth_rejection(monkeypatch) -> None:
    monkeypatch.setenv("SIDECAR_TOKEN", "secret")

    response = TestClient(main.app).post("/v1/lore/search", json={"query": "Tonto"})

    assert response.status_code == 401


def test_lore_search_route_empty_results(monkeypatch) -> None:
    monkeypatch.setattr(main, "lore_search", FakeLoreSearch())
    monkeypatch.delenv("SIDECAR_TOKEN", raising=False)

    response = TestClient(main.app).post("/v1/lore/search", json={"query": "nothing"})

    assert response.status_code == 200
    assert response.json() == {"snippets": []}


def test_lore_search_route_backend_error(monkeypatch) -> None:
    monkeypatch.setattr(main, "lore_search", FakeLoreSearch(error=RuntimeError("qdrant unreachable")))
    monkeypatch.delenv("SIDECAR_TOKEN", raising=False)

    response = TestClient(main.app).post("/v1/lore/search", json={"query": "Tonto"})

    assert response.status_code == 503
    assert response.json()["detail"] == "qdrant unreachable"
