from __future__ import annotations

import os
from pathlib import Path


def load_project_env() -> None:
    """Load a simple KEY=VALUE .env file without adding a runtime dependency."""
    for directory in [Path.cwd(), *Path.cwd().parents]:
        env_path = directory / ".env"
        if env_path.exists():
            _load_file(env_path)
            return


def _load_file(path: Path) -> None:
    for raw in path.read_text().splitlines():
        line = raw.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        key = key.strip()
        value = value.strip().strip('"').strip("'")
        if key and key not in os.environ:
            os.environ[key] = value

