import os
import time
import requests
import pytest

BASE_URL = os.environ.get("NOTIFICATION_API_URL", "http://notification_api:18090")


def test_health():
    r = requests.get(f"{BASE_URL}/health", timeout=5)
    assert r.status_code == 200


def test_send_email_notification():
    payload = {
        "template": "welcome_email",
        "recipient": "test@example.com",
        "channel": "email",
        "vars": {"name": "Smoke Tester"},
    }
    r = requests.post(f"{BASE_URL}/api/v1/notifications", json=payload, timeout=5)
    assert r.status_code == 201
    data = r.json()
    assert data["status"] == "pending"
    assert "id" in data


def test_notification_dispatched():
    payload = {
        "template": "sms_otp",
        "recipient": "+15550001234",
        "channel": "sms",
        "vars": {"otp": "482910"},
    }
    r = requests.post(f"{BASE_URL}/api/v1/notifications", json=payload, timeout=5)
    assert r.status_code == 201
    notification_id = r.json()["id"]

    for _ in range(10):
        time.sleep(1)
        r = requests.get(f"{BASE_URL}/api/v1/notifications/{notification_id}", timeout=5)
        if r.json().get("status") == "sent":
            return

    pytest.fail("Notification was not dispatched within 10 seconds.")


def test_unknown_template_rejected():
    payload = {
        "template": "does_not_exist",
        "recipient": "x@example.com",
        "channel": "email",
        "vars": {},
    }
    r = requests.post(f"{BASE_URL}/api/v1/notifications", json=payload, timeout=5)
    assert r.status_code == 422
