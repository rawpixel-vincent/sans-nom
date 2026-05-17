from __future__ import annotations

import argparse
from pathlib import Path

from .lore_index import embed_documents, ensure_collection
from .lore_ops import qdrant_url
from .models import StoryIndexDocument, StoryIndexIngestRequest, StoryIndexIngestResponse


def ingest_story_index(req: StoryIndexIngestRequest) -> StoryIndexIngestResponse:
    docs = [doc for doc in req.documents if doc.text.strip()]
    if not docs:
        return StoryIndexIngestResponse(collection=req.collection, count=0, replaced=req.replace)

    vectors = embed_documents([doc.text for doc in docs], fake=req.fake_embeddings)
    if not vectors:
        raise RuntimeError("no story documents to ingest")

    from qdrant_client import QdrantClient
    from qdrant_client.http.models import Distance, PointStruct, VectorParams

    client = QdrantClient(url=qdrant_url(), check_compatibility=False)
    ensure_collection(
        client,
        collection=req.collection,
        vector_size=len(vectors[0]),
        replace=req.replace,
        vector_params=VectorParams,
        distance=Distance,
    )
    points = [
        PointStruct(id=doc.point_id, vector=vector, payload=doc.qdrant_payload())
        for doc, vector in zip(docs, vectors, strict=True)
    ]
    client.upsert(collection_name=req.collection, points=points)
    return StoryIndexIngestResponse(collection=req.collection, count=len(points), replaced=req.replace)


def read_story_documents(path: Path) -> list[StoryIndexDocument]:
    docs = []
    for line_no, line in enumerate(path.read_text().splitlines(), start=1):
        if not line.strip():
            continue
        try:
            docs.append(StoryIndexDocument.model_validate_json(line))
        except Exception as exc:
            raise ValueError(f"{path}:{line_no}: {exc}") from exc
    return docs


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="story-index", description="Ingest generated story index documents.")
    sub = parser.add_subparsers(dest="cmd", required=True)
    ingest = sub.add_parser("ingest", help="Embed and upsert story documents to Qdrant.")
    ingest.add_argument("--documents", required=True, help="Path to data/stories/<id>/index/documents.jsonl")
    ingest.add_argument("--collection", required=True)
    ingest.add_argument("--replace", action="store_true")
    ingest.add_argument("--fake-embeddings", action="store_true")
    args = parser.parse_args(argv)

    if args.cmd == "ingest":
        resp = ingest_story_index(StoryIndexIngestRequest(
            collection=args.collection,
            documents=read_story_documents(Path(args.documents)),
            replace=args.replace,
            fake_embeddings=args.fake_embeddings,
        ))
        print(f"ingested {resp.count} docs into {resp.collection}")
        return 0
    raise AssertionError(args.cmd)
