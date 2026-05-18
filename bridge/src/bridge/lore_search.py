from __future__ import annotations

from typing import Any

from .lore_index import DEFAULT_COLLECTION, float_vector, query_points
from .lore_ops import qdrant_url, voyage_api_key, voyage_model
from .models import LoreSearchRequest, LoreSearchResponse, LoreSnippet

MAX_LORE_K = 20
TEXT_PREVIEW_CHARS = 1800


class LoreSearchService:
    def __init__(self, collection: str = DEFAULT_COLLECTION) -> None:
        self.collection = collection

    def search(self, req: LoreSearchRequest) -> LoreSearchResponse:
        key = voyage_api_key()
        if not key:
            raise RuntimeError("SANS_NOM_VOYAGE_API_KEY or VOYAGE_API_KEY is required")

        from voyageai.client import Client as VoyageClient
        from qdrant_client import QdrantClient

        query = req.query.strip()
        if not query:
            return LoreSearchResponse(snippets=[])

        vector = float_vector(VoyageClient(api_key=key).embed(
            [query],
            model=voyage_model(),
            input_type="query",
        ).embeddings[0])
        client = QdrantClient(url=qdrant_url(), check_compatibility=False)
        hits = query_points(
            client,
            req.collection or self.collection,
            vector,
            limit=clamp_k(req.k),
            query_filter=build_filter(req),
        )
        snippets = [snippet_from_hit(hit) for hit in hits]
        snippets.sort(key=lambda snippet: snippet.score, reverse=True)
        return LoreSearchResponse(snippets=snippets)


def clamp_k(k: int) -> int:
    if k < 1:
        return 1
    if k > MAX_LORE_K:
        return MAX_LORE_K
    return k


def build_filter(req: LoreSearchRequest) -> Any | None:
    conditions = []
    for field, values in (
        ("kind", req.kind),
        ("peer_id", req.peer_id),
        ("character_id", req.character_id),
        ("volume", req.volume),
    ):
        if values:
            conditions.append(field_condition(field, values))
    if not conditions:
        return None

    from qdrant_client.http.models import Filter

    return Filter(must=conditions)


def field_condition(field: str, values: list[str] | list[int]) -> Any:
    from qdrant_client.http.models import FieldCondition, MatchAny, MatchValue

    if len(values) == 1:
        return FieldCondition(key=field, match=MatchValue(value=values[0]))
    return FieldCondition(key=field, match=MatchAny(any=values))


def snippet_from_hit(hit: Any) -> LoreSnippet:
    payload = hit.payload or {}
    text = str(payload.get("text") or "")
    response_payload = dict(payload)
    response_payload.pop("text", None)
    return LoreSnippet(
        doc_id=str(payload.get("doc_id") or payload.get("id") or hit.id),
        text=preview_text(text),
        source_path=str(payload.get("source_path") or payload.get("source") or ""),
        score=float(hit.score or 0),
        kind=str(payload.get("kind") or ""),
        page_id=optional_str(payload.get("page_id")),
        peer_id=optional_str(payload.get("peer_id")),
        character_id=optional_str(payload.get("character_id")),
        volume=optional_int(payload.get("volume")),
        payload=response_payload,
    )


def preview_text(text: str) -> str:
    if len(text) <= TEXT_PREVIEW_CHARS:
        return text
    return text[:TEXT_PREVIEW_CHARS].rstrip() + "..."


def optional_str(value: Any) -> str | None:
    if value is None:
        return None
    text = str(value)
    return text or None


def optional_int(value: Any) -> int | None:
    if value is None:
        return None
    return int(value)
