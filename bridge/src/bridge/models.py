from __future__ import annotations

from typing import Any

from pydantic import BaseModel, Field, field_validator


class PlayerDecision(BaseModel):
    choice_id: str | None = None
    free_text: str | None = None


class ActorMessage(BaseModel):
    actor_id: str
    interior: str | None = None
    dialogue: str | None = None
    action: str | None = None
    surface: str | None = None
    witnesses: list[str] = Field(default_factory=list)


class CanonEvent(BaseModel):
    peer_id: str
    content: str
    source: str
    witnesses: list[str] = Field(default_factory=list)


class CommitRequest(BaseModel):
    workspace_id: str
    session_id: str
    packet_id: str
    node_id: str
    player_decision: PlayerDecision
    final_prose: str
    actor_messages: list[ActorMessage] = Field(default_factory=list)
    events: list[CanonEvent] = Field(default_factory=list)
    witnesses: list[str] = Field(default_factory=list)


class SceneContextRequest(BaseModel):
    workspace_id: str
    session_id: str
    player_peer_id: str = "sans-nom"
    recent_scene_count: int = 3
    representations: list[str] = Field(default_factory=list)
    search_query: str | None = None


class SceneContextResponse(BaseModel):
    workspace_id: str
    session_id: str
    recent_scenes: list[str] = Field(default_factory=list)
    representations: dict[str, str] = Field(default_factory=dict)
    peer_cards: dict[str, list[str]] = Field(default_factory=dict)


class PeerCardPatch(BaseModel):
    facts: list[str]
    replace: bool = False
    target: str | None = None


class RepresentationRequest(BaseModel):
    workspace_id: str
    observer: str
    observed: str
    search_query: str | None = None
    top_k: int = 10


class RepresentationResponse(BaseModel):
    observer: str
    observed: str
    representation: str


class LoreSnippet(BaseModel):
    doc_id: str
    text: str
    source_path: str
    score: float = 0.0
    kind: str
    page_id: str | None = None
    peer_id: str | None = None
    character_id: str | None = None
    volume: int | None = None
    payload: dict[str, Any] = Field(default_factory=dict)


class LoreSearchRequest(BaseModel):
    collection: str | None = None
    query: str
    k: int = 5
    kind: list[str] = Field(default_factory=list)
    peer_id: list[str] = Field(default_factory=list)
    character_id: list[str] = Field(default_factory=list)
    volume: list[int] = Field(default_factory=list)

    @field_validator("kind", "peer_id", "character_id", mode="before")
    @classmethod
    def coerce_string_filter(cls, value: Any) -> list[str]:
        if value is None:
            return []
        if isinstance(value, str):
            return [value]
        return value

    @field_validator("volume", mode="before")
    @classmethod
    def coerce_volume_filter(cls, value: Any) -> list[int]:
        if value is None:
            return []
        if isinstance(value, int):
            return [value]
        return value


class LoreSearchResponse(BaseModel):
    snippets: list[LoreSnippet] = Field(default_factory=list)


class StoryIndexDocument(BaseModel):
    doc_id: str
    point_id: str
    kind: str
    source_path: str
    source_sha256: str
    text_sha256: str
    text: str
    story_id: str | None = None
    packet_id: str | None = None
    node_id: str | None = None
    turn: int | None = None
    heading: str | None = None
    payload: dict[str, Any] = Field(default_factory=dict)

    def qdrant_payload(self) -> dict[str, Any]:
        data = self.model_dump(exclude_none=True)
        payload = dict(data.pop("payload", {}))
        payload.update(data)
        return payload


class StoryIndexIngestRequest(BaseModel):
    collection: str
    documents: list[StoryIndexDocument]
    replace: bool = False
    fake_embeddings: bool = False


class StoryIndexIngestResponse(BaseModel):
    collection: str
    count: int
    replaced: bool = False


class StoryDeleteRequest(BaseModel):
    workspace_id: str
    collection: str


class StoryDeleteResponse(BaseModel):
    workspace_id: str
    collection: str
    qdrant_collection_deleted: bool = False
    honcho_workspace_deleted: bool = False
    honcho_sessions_deleted: int = 0
    honcho_live: bool = False
