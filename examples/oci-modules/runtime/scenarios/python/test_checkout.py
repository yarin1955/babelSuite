import os


def test_checkout_smoke():
    assert os.environ.get("BASE_URL")
