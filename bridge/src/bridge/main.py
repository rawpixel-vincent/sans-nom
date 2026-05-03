from __future__ import annotations

import os

from fastapi import Depends, FastAPI, Header, HTTPException

from .honcho_ops import HonchoOps
from .lore_ops import qdrant_url, voyage_api_key, voyage_model
from .models import (
    CommitRequest,
    PeerCardPatch,
    RepresentationRequest,
    RepresentationResponse,
    SceneContextRequest,
    SceneContextResponse,
)

app = FastAPI(title="Sans-Nom Phase 0 Bridge", version="0.1.0")
ops = HonchoOps()


def authorize(authorization: str | None = Header(default=None)) -> None:
    token = os.getenv("SIDECAR_TOKEN")
    if not token:
        return
    if authorization != f"Bearer {token}":
        raise HTTPException(status_code=401, detail="invalid sidecar token")


@app.get("/v1/health")
def health() -> dict[str, object]:
    return {
        "status": "ok",
        "honcho_live": ops.live_enabled,
        "voyage_configured": bool(voyage_api_key()),
        "voyage_model": voyage_model(),
        "qdrant_url": qdrant_url(),
    }


@app.post("/v1/scene/context", response_model=SceneContextResponse, dependencies=[Depends(authorize)])
def scene_context(req: SceneContextRequest) -> SceneContextResponse:
    return ops.scene_context(req)


@app.post("/v1/scene/commit", dependencies=[Depends(authorize)])
def scene_commit(req: CommitRequest) -> dict[str, str]:
    forbidden = {"writer_draft", "unused_branch", "actor_private"}
    for event in req.events:
        if event.source in forbidden:
            raise HTTPException(status_code=422, detail=f"forbidden commit source: {event.source}")
        if not event.witnesses:
            raise HTTPException(status_code=422, detail="canon events require witnesses")
    return ops.commit_scene(req)


@app.get("/v1/peer/{peer_id}/card", dependencies=[Depends(authorize)])
def get_card(peer_id: str, workspace_id: str, target: str | None = None) -> dict[str, object]:
    return {"peer_id": peer_id, "target": target, "facts": ops.get_card(workspace_id, peer_id, target)}


@app.patch("/v1/peer/{peer_id}/card", dependencies=[Depends(authorize)])
def patch_card(peer_id: str, workspace_id: str, patch: PeerCardPatch) -> dict[str, object]:
    return {"peer_id": peer_id, "target": patch.target, "facts": ops.patch_card(workspace_id, peer_id, patch)}


@app.post("/v1/repr", response_model=RepresentationResponse, dependencies=[Depends(authorize)])
def representation(req: RepresentationRequest) -> RepresentationResponse:
    return ops.representation(req)

