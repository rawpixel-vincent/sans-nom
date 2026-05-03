from __future__ import annotations

from pydantic import BaseModel, Field


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
    id: str
    text: str
    source: str
    score: float = 0.0


class LoreSearchRequest(BaseModel):
    query: str
    k: int = 5


class LoreSearchResponse(BaseModel):
    snippets: list[LoreSnippet] = Field(default_factory=list)

