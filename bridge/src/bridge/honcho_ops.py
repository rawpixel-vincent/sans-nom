from __future__ import annotations

import os
from collections import defaultdict
from typing import Any

from .env import load_project_env
from .models import (
    CommitRequest,
    PeerCardPatch,
    RepresentationRequest,
    RepresentationResponse,
    SceneContextRequest,
    SceneContextResponse,
)

load_project_env()


class HonchoOps:
    """Thin Honcho SDK wrapper with an in-memory dev fallback.

    Live hosted Honcho is used when the SDK can be imported and an API key is
    configured. The fallback keeps the sidecar useful for local contract tests.
    """

    def __init__(self) -> None:
        self.api_key = os.getenv("SANS_NOM_HONCHO_API_KEY") or os.getenv("HONCHO_API_KEY")
        self.environment = os.getenv("HONCHO_ENVIRONMENT", "production")
        self.base_url = os.getenv("HONCHO_BASE_URL")
        self._cards: dict[str, list[str]] = defaultdict(list)
        self._messages: dict[str, list[CommitRequest]] = defaultdict(list)

    @property
    def live_enabled(self) -> bool:
        return bool(self.api_key)

    def _client(self, workspace_id: str) -> Any:
        from honcho import Honcho

        kwargs: dict[str, Any] = {
            "workspace_id": workspace_id,
            "api_key": self.api_key,
            "environment": self.environment,
        }
        if self.base_url:
            kwargs["base_url"] = self.base_url
        return Honcho(**kwargs)

    def scene_context(self, req: SceneContextRequest) -> SceneContextResponse:
        if not self.live_enabled:
            recent = [m.final_prose for m in self._messages[req.session_id][-req.recent_scene_count :]]
            return SceneContextResponse(
                workspace_id=req.workspace_id,
                session_id=req.session_id,
                recent_scenes=recent,
                representations={key: "" for key in req.representations},
            )

        honcho = self._client(req.workspace_id)
        session = honcho.session(req.session_id)
        context = session.context(
            summary=True,
            tokens=2000,
            peer_target=req.player_peer_id,
            peer_perspective="narrator",
            search_query=req.search_query,
        )
        recent = [getattr(msg, "content", "") for msg in getattr(context, "messages", [])]
        return SceneContextResponse(
            workspace_id=req.workspace_id,
            session_id=req.session_id,
            recent_scenes=recent,
            representations={req.player_peer_id: getattr(context, "peer_representation", "") or ""},
            peer_cards={req.player_peer_id: getattr(context, "peer_card", []) or []},
        )

    def commit_scene(self, req: CommitRequest) -> dict[str, str]:
        self._messages[req.session_id].append(req)
        if not self.live_enabled:
            return {"status": "committed-dev"}

        from honcho.api_types import SessionPeerConfig

        honcho = self._client(req.workspace_id)
        session = honcho.session(req.session_id)
        peer_ids = set(req.witnesses)
        peer_ids.update(event.peer_id for event in req.events)
        peers = [honcho.peer(peer_id) for peer_id in sorted(peer_ids)]
        session.add_peers([(peer, SessionPeerConfig(observe_others=True)) for peer in peers])

        messages = []
        for event in req.events:
            peer = honcho.peer(event.peer_id)
            messages.append(peer.message(event.content, metadata={
                "packet_id": req.packet_id,
                "node_id": req.node_id,
                "source": event.source,
                "witnesses": event.witnesses,
            }))
        if messages:
            session.add_messages(messages)
        return {"status": "committed"}

    def get_card(self, workspace_id: str, peer_id: str, target: str | None = None) -> list[str]:
        key = self._card_key(workspace_id, peer_id, target)
        if not self.live_enabled:
            return list(self._cards[key])
        peer = self._client(workspace_id).peer(peer_id)
        return list(peer.get_card(target=target) or [])

    def patch_card(self, workspace_id: str, peer_id: str, patch: PeerCardPatch) -> list[str]:
        current = [] if patch.replace else self.get_card(workspace_id, peer_id, patch.target)
        updated = current + patch.facts
        key = self._card_key(workspace_id, peer_id, patch.target)
        self._cards[key] = updated
        if self.live_enabled:
            peer = self._client(workspace_id).peer(peer_id)
            peer.set_card(updated, target=patch.target)
        return updated

    def representation(self, req: RepresentationRequest) -> RepresentationResponse:
        if not self.live_enabled:
            return RepresentationResponse(
                observer=req.observer,
                observed=req.observed,
                representation="",
            )
        peer = self._client(req.workspace_id).peer(req.observer)
        context = peer.context(
            target=req.observed,
            search_query=req.search_query,
            search_top_k=req.top_k,
        )
        return RepresentationResponse(
            observer=req.observer,
            observed=req.observed,
            representation=getattr(context, "representation", "") or "",
        )

    @staticmethod
    def _card_key(workspace_id: str, peer_id: str, target: str | None) -> str:
        return f"{workspace_id}:{peer_id}:{target or ''}"
