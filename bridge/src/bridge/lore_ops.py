from __future__ import annotations

import os

from .env import load_project_env

load_project_env()

def voyage_api_key() -> str | None:
    return os.getenv("SANS_NOM_VOYAGE_API_KEY") or os.getenv("VOYAGE_API_KEY")


def qdrant_url() -> str:
    return os.getenv("QDRANT_URL", "http://127.0.0.1:6333")


def voyage_model() -> str:
    return os.getenv("SANS_NOM_VOYAGE_MODEL", "voyage-4")
