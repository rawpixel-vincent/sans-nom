from __future__ import annotations

from bridge.honcho_ops import HonchoOps, honcho_peer_id
from bridge.models import CanonEvent, CommitRequest, PeerCardPatch, RepresentationRequest, SceneContextRequest


class NotFoundError(Exception):
    pass


class FakeSession:
    def __init__(self, client: "FakeClient | None" = None) -> None:
        self.client = client

    def context(self, **kwargs: object) -> object:
        if self.client is not None:
            self.client.session_context_kwargs.append(kwargs)
        raise NotFoundError("missing peer")

    def add_peers(self, peers: list[tuple["FakePeer", object]]) -> None:
        if self.client is not None:
            self.client.added_peer_ids.extend(peer.peer_id for peer, _ in peers)

    def add_messages(self, messages: list[dict[str, object]]) -> None:
        if self.client is not None:
            self.client.messages.extend(messages)

    def delete(self) -> None:
        if self.client is not None:
            self.client.deleted_sessions += 1


class FakePeer:
    def __init__(self, peer_id: str = "", client: "FakeClient | None" = None) -> None:
        self.peer_id = peer_id
        self.client = client

    def get_card(self, target: str | None = None) -> list[str]:
        if self.client is not None:
            self.client.get_card_targets.append(target)
        raise NotFoundError("missing peer")

    def set_card(self, facts: list[str], target: str | None = None) -> None:
        if self.client is not None:
            self.client.set_cards.append((self.peer_id, list(facts), target))

    def context(self, **kwargs: object) -> object:
        if self.client is not None:
            self.client.peer_context_kwargs.append(kwargs)
        raise NotFoundError("missing peer")

    def message(self, content: str, metadata: dict[str, object]) -> dict[str, object]:
        return {"peer_id": self.peer_id, "content": content, "metadata": metadata}


class FakeClient:
    def __init__(self) -> None:
        self.peer_ids: list[str] = []
        self.added_peer_ids: list[str] = []
        self.messages: list[dict[str, object]] = []
        self.get_card_targets: list[str | None] = []
        self.set_cards: list[tuple[str, list[str], str | None]] = []
        self.peer_context_kwargs: list[dict[str, object]] = []
        self.session_context_kwargs: list[dict[str, object]] = []
        self.deleted_sessions = 0
        self.deleted_workspaces: list[str] = []

    def session(self, session_id: str) -> FakeSession:
        return FakeSession(self)

    def peer(self, peer_id: str) -> FakePeer:
        self.peer_ids.append(peer_id)
        return FakePeer(peer_id, self)

    def sessions(self) -> list[FakeSession]:
        return [FakeSession(self), FakeSession(self)]

    def delete_workspace(self, workspace_id: str) -> None:
        self.deleted_workspaces.append(workspace_id)


def test_scene_context_returns_empty_context_for_missing_live_honcho_peer(monkeypatch) -> None:
    ops = HonchoOps()
    ops.api_key = "test-key"
    client = FakeClient()
    monkeypatch.setattr(ops, "_client", lambda workspace_id: client)

    resp = ops.scene_context(
        SceneContextRequest(
            workspace_id="phase0-dev",
            session_id="session",
            player_peer_id="player:saren_iku_no",
            representations=["narrator:player:saren_iku_no"],
        )
    )

    assert resp.recent_scenes == []
    assert resp.representations == {"narrator:player:saren_iku_no": ""}
    assert client.session_context_kwargs[0]["peer_target"] == "player-saren_iku_no"


def test_get_card_returns_empty_for_missing_live_honcho_peer(monkeypatch) -> None:
    ops = HonchoOps()
    ops.api_key = "test-key"
    client = FakeClient()
    monkeypatch.setattr(ops, "_client", lambda workspace_id: client)

    assert ops.get_card("phase0-dev", "player:saren_iku_no", "actor:mother") == []
    assert client.peer_ids == ["player-saren_iku_no"]
    assert client.get_card_targets == ["actor-mother"]


def test_patch_card_normalizes_live_honcho_peer_id_and_target(monkeypatch) -> None:
    ops = HonchoOps()
    ops.api_key = "test-key"
    client = FakeClient()
    monkeypatch.setattr(ops, "_client", lambda workspace_id: client)

    facts = ops.patch_card(
        "phase0-dev",
        "player:saren_iku_no",
        patch=PeerCardPatch(
            facts=["Name: Saren"],
            replace=True,
            target="actor:mother",
        ),
    )

    assert facts == ["Name: Saren"]
    assert client.peer_ids == ["player-saren_iku_no"]
    assert client.set_cards == [("player-saren_iku_no", ["Name: Saren"], "actor-mother")]


def test_dev_card_store_uses_same_normalized_key_transparently() -> None:
    ops = HonchoOps()

    ops.patch_card(
        "phase0-dev",
        "player:saren_iku_no",
        patch=PeerCardPatch(
            facts=["Name: Saren"],
            replace=True,
            target="actor:mother",
        ),
    )

    assert ops.get_card("phase0-dev", "player:saren_iku_no", "actor:mother") == ["Name: Saren"]


def test_representation_returns_empty_for_missing_live_honcho_peer(monkeypatch) -> None:
    ops = HonchoOps()
    ops.api_key = "test-key"
    client = FakeClient()
    monkeypatch.setattr(ops, "_client", lambda workspace_id: client)

    resp = ops.representation(
        RepresentationRequest(
            workspace_id="phase0-dev",
            observer="actor:mother",
            observed="player:saren_iku_no",
        )
    )

    assert resp.representation == ""
    assert client.peer_ids == ["actor-mother"]
    assert client.peer_context_kwargs[0]["target"] == "player-saren_iku_no"


def test_commit_scene_normalizes_human_witness_labels_for_live_honcho(monkeypatch) -> None:
    ops = HonchoOps()
    ops.api_key = "test-key"
    client = FakeClient()
    monkeypatch.setattr(ops, "_client", lambda workspace_id: client)

    resp = ops.commit_scene(
        CommitRequest(
            workspace_id="story-one",
            session_id="main",
            packet_id="packet",
            node_id="node",
            player_decision={},
            final_prose="prose",
            witnesses=["sans-nom", "Admiral-Haruspex Veyr"],
            events=[
                CanonEvent(
                    peer_id="narrator",
                    content="The admiral watched.",
                    source="narrator",
                    witnesses=["sans-nom", "Admiral-Haruspex Veyr"],
                )
            ],
        )
    )

    assert resp == {"status": "committed"}
    assert client.added_peer_ids == ["narrator"]
    assert all(" " not in peer_id for peer_id in client.peer_ids)
    metadata = client.messages[0]["metadata"]
    assert metadata["witnesses"] == ["sans-nom", "admiral-haruspex-veyr"]
    assert metadata["witness_labels"] == ["sans-nom", "Admiral-Haruspex Veyr"]


def test_commit_scene_does_not_add_every_witness_as_observer(monkeypatch) -> None:
    ops = HonchoOps()
    ops.api_key = "test-key"
    client = FakeClient()
    monkeypatch.setattr(ops, "_client", lambda workspace_id: client)

    labels = [f"Witness Group {index}" for index in range(14)]
    ops.commit_scene(
        CommitRequest(
            workspace_id="story-one",
            session_id="main",
            packet_id="packet",
            node_id="node",
            player_decision={},
            final_prose="prose",
            witnesses=labels,
            events=[
                CanonEvent(
                    peer_id="narrator",
                    content="A crowded public event happened.",
                    source="narrator",
                    witnesses=labels,
                )
            ],
        )
    )

    assert client.added_peer_ids == ["narrator"]


def test_commit_scene_adds_explicit_observers_without_witness_fanout(monkeypatch) -> None:
    ops = HonchoOps()
    ops.api_key = "test-key"
    client = FakeClient()
    monkeypatch.setattr(ops, "_client", lambda workspace_id: client)

    labels = ["sans-nom", "Crowd of sealed cousins", "Archive staff"]
    ops.commit_scene(
        CommitRequest(
            workspace_id="story-one",
            session_id="main",
            packet_id="packet",
            node_id="node",
            player_decision={},
            final_prose="prose",
            witnesses=labels,
            observer_peer_ids=["sans-nom", "narrator", "iri itta"],
            events=[
                CanonEvent(
                    peer_id="narrator",
                    content="Iri witnessed silently.",
                    source="narrator",
                    witnesses=labels,
                )
            ],
        )
    )

    assert client.added_peer_ids == ["iri-itta", "narrator", "sans-nom"]
    assert "crowd-of-sealed-cousins" not in client.added_peer_ids
    assert "archive-staff" not in client.added_peer_ids


def test_honcho_peer_id_slugifies_invalid_labels() -> None:
    assert honcho_peer_id("Admiral-Haruspex Veyr") == "admiral-haruspex-veyr"
    assert honcho_peer_id("  ") == "unknown-peer"


def test_delete_workspace_deletes_sessions_first(monkeypatch) -> None:
    ops = HonchoOps()
    ops.api_key = "test-key"
    client = FakeClient()
    monkeypatch.setattr(ops, "_client", lambda workspace_id: client)

    deleted, session_count = ops.delete_workspace("story-one")

    assert deleted is True
    assert session_count == 2
    assert client.deleted_sessions == 2
    assert client.deleted_workspaces == ["story-one"]
