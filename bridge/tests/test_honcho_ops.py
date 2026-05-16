from __future__ import annotations

from bridge.honcho_ops import HonchoOps
from bridge.models import RepresentationRequest, SceneContextRequest


class NotFoundError(Exception):
    pass


class FakeSession:
    def context(self, **_: object) -> object:
        raise NotFoundError("missing peer")


class FakePeer:
    def get_card(self, target: str | None = None) -> list[str]:
        raise NotFoundError("missing peer")

    def context(self, **_: object) -> object:
        raise NotFoundError("missing peer")


class FakeClient:
    def session(self, session_id: str) -> FakeSession:
        return FakeSession()

    def peer(self, peer_id: str) -> FakePeer:
        return FakePeer()


def live_ops_with_missing_peers(monkeypatch) -> HonchoOps:
    ops = HonchoOps()
    ops.api_key = "test-key"
    monkeypatch.setattr(ops, "_client", lambda workspace_id: FakeClient())
    return ops


def test_scene_context_returns_empty_context_for_missing_live_honcho_peer(monkeypatch) -> None:
    ops = live_ops_with_missing_peers(monkeypatch)

    resp = ops.scene_context(
        SceneContextRequest(
            workspace_id="phase0-dev",
            session_id="session",
            player_peer_id="sans-nom",
            representations=["narrator:sans-nom"],
        )
    )

    assert resp.recent_scenes == []
    assert resp.representations == {"narrator:sans-nom": ""}


def test_get_card_returns_empty_for_missing_live_honcho_peer(monkeypatch) -> None:
    ops = live_ops_with_missing_peers(monkeypatch)

    assert ops.get_card("phase0-dev", "tonto") == []


def test_representation_returns_empty_for_missing_live_honcho_peer(monkeypatch) -> None:
    ops = live_ops_with_missing_peers(monkeypatch)

    resp = ops.representation(
        RepresentationRequest(
            workspace_id="phase0-dev",
            observer="tonto",
            observed="sans-nom",
        )
    )

    assert resp.representation == ""
