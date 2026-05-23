from __future__ import annotations

import os


def pytest_configure() -> None:
	if os.getenv("SANS_NOM_TEST_LIVE_HONCHO") == "1":
		return
	os.environ["SANS_NOM_HONCHO_API_KEY"] = ""
	os.environ["HONCHO_API_KEY"] = ""
