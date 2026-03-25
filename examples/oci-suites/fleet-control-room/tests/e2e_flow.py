import os

import requests


def test_health():
    base_url = os.environ.get("FLEET_API_URL", "http://fleet-api:8080")
    response = requests.get(f"{base_url}/health", timeout=5)
    response.raise_for_status()

    payload = response.json()
    assert payload["ok"] is True
    assert payload["service"] == "fleet-api"


def test_summary_window():
    base_url = os.environ.get("FLEET_API_URL", "http://fleet-api:8080")
    replay_window = int(os.environ.get("REPLAY_WINDOW_MINUTES", "15"))

    response = requests.get(f"{base_url}/api/fleet/summary", timeout=5)
    response.raise_for_status()

    payload = response.json()
    assert payload["devices"] >= 1
    assert payload["activeAlerts"] >= 0
    assert payload["replayWindowMinutes"] == replay_window
