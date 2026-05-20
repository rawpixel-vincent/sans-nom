from __future__ import annotations

import os
import re
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

PEER_ID_RE = re.compile(r"[^a-z0-9_-]+")


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
        player_peer_id = honcho_peer_id(req.player_peer_id)
        try:
            context = session.context(
                summary=True,
                tokens=2000,
                peer_target=player_peer_id,
                peer_perspective="narrator",
                search_query=req.search_query,
            )
        except Exception as exc:
            if self._is_not_found(exc):
                return SceneContextResponse(
                    workspace_id=req.workspace_id,
                    session_id=req.session_id,
                    recent_scenes=[],
                    representations={key: "" for key in req.representations},
                )
            raise
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
        peer_ids = {honcho_peer_id(event.peer_id) for event in req.events}
        peers = [honcho.peer(peer_id) for peer_id in sorted(peer_ids)]
        if peers:
            session.add_peers([(peer, SessionPeerConfig(observe_others=True, observe_me=True)) for peer in peers])

        messages = []
        for event in req.events:
            peer_id = honcho_peer_id(event.peer_id)
            witness_peer_ids = [honcho_peer_id(witness) for witness in event.witnesses]
            metadata: dict[str, Any] = {
                "packet_id": req.packet_id,
                "node_id": req.node_id,
                "source": event.source,
                "witnesses": witness_peer_ids,
            }
            if peer_id != event.peer_id:
                metadata["raw_peer_id"] = event.peer_id
            if witness_peer_ids != event.witnesses:
                metadata["witness_labels"] = event.witnesses
            peer = honcho.peer(peer_id)
            messages.append(peer.message(event.content, metadata=metadata))
        if messages:
            session.add_messages(messages)
        return {"status": "committed"}

    def get_card(self, workspace_id: str, peer_id: str, target: str | None = None) -> list[str]:
        safe_peer_id = honcho_peer_id(peer_id)
        safe_target = honcho_peer_id(target) if target else None
        key = self._card_key(workspace_id, safe_peer_id, safe_target)
        if not self.live_enabled:
            return list(self._cards[key])
        peer = self._client(workspace_id).peer(safe_peer_id)
        try:
            return list(peer.get_card(target=safe_target) or [])
        except Exception as exc:
            if self._is_not_found(exc):
                return []
            raise

    def patch_card(self, workspace_id: str, peer_id: str, patch: PeerCardPatch) -> list[str]:
        safe_peer_id = honcho_peer_id(peer_id)
        safe_target = honcho_peer_id(patch.target) if patch.target else None
        current = [] if patch.replace else self.get_card(workspace_id, peer_id, patch.target)
        updated = current + patch.facts
        key = self._card_key(workspace_id, safe_peer_id, safe_target)
        self._cards[key] = updated
        if self.live_enabled:
            peer = self._client(workspace_id).peer(safe_peer_id)
            peer.set_card(updated, target=safe_target)
        return updated

    def delete_workspace(self, workspace_id: str) -> tuple[bool, int]:
        prefix = f"{workspace_id}:"
        for key in list(self._cards):
            if key.startswith(prefix):
                del self._cards[key]
        self._messages.pop("main", None)
        if not self.live_enabled:
            return False, 0
        try:
            honcho = self._client(workspace_id)
            sessions_deleted = 0
            for session in honcho.sessions():
                session.delete()
                sessions_deleted += 1
            honcho.delete_workspace(workspace_id)
        except Exception as exc:
            if self._is_not_found(exc):
                return False, 0
            raise
        return True, sessions_deleted

    def representation(self, req: RepresentationRequest) -> RepresentationResponse:
        if not self.live_enabled:
            return RepresentationResponse(
                observer=req.observer,
                observed=req.observed,
                representation="",
            )
        safe_observer = honcho_peer_id(req.observer)
        safe_observed = honcho_peer_id(req.observed)
        peer = self._client(req.workspace_id).peer(safe_observer)
        try:
            context = peer.context(
                target=safe_observed,
                search_query=req.search_query,
                search_top_k=req.top_k,
            )
        except Exception as exc:
            if self._is_not_found(exc):
                return RepresentationResponse(
                    observer=req.observer,
                    observed=req.observed,
                    representation="",
                )
            raise
        return RepresentationResponse(
            observer=req.observer,
            observed=req.observed,
            representation=getattr(context, "representation", "") or "",
        )

    @staticmethod
    def _card_key(workspace_id: str, peer_id: str, target: str | None) -> str:
        return f"{workspace_id}:{peer_id}:{target or ''}"

    @staticmethod
    def _is_not_found(exc: Exception) -> bool:
        return exc.__class__.__name__ == "NotFoundError"


def honcho_peer_id(value: str) -> str:
    peer_id = PEER_ID_RE.sub("-", str(value).strip().lower()).strip("-_")
    return peer_id or "unknown-peer"
