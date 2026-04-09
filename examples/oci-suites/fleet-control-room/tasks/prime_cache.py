from pathlib import Path

SUITE_ID = "fleet-control-room"
SCRIPT_NAME = "prime_cache.py"

def main() -> None:
    print(f"running {SCRIPT_NAME} for {SUITE_ID}")
    print(Path('.').resolve())

if __name__ == "__main__":
    main()
