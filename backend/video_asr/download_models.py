"""Install the pinned public models once; never run during an import request."""
from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import tempfile
import urllib.request


def matches(path: Path, item: dict) -> bool:
    if path.is_symlink() or not path.is_file() or path.stat().st_size != item["size"]:
        return False
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1 << 20), b""):
            digest.update(block)
    return digest.hexdigest() == item["sha256"]


def install(manifest: Path, destination: Path) -> None:
    files = json.loads(manifest.read_text())["files"]
    destination.mkdir(parents=True, exist_ok=True)
    for item in files:
        name = item["name"]
        if Path(name).name != name or not item["url"].startswith("https://"):
            raise ValueError("invalid model manifest")
        target = destination / name
        if matches(target, item):
            print(f"verified {name}", flush=True)
            continue
        temporary = None
        try:
            with tempfile.NamedTemporaryFile(dir=destination, prefix=".download-", delete=False) as out:
                temporary = Path(out.name)
                request = urllib.request.Request(item["url"], headers={"User-Agent": "ArreCook-model-setup/1.0"})
                with urllib.request.urlopen(request, timeout=60) as response:
                    count = 0
                    while block := response.read(1 << 20):
                        count += len(block)
                        if count > item["size"]:
                            raise ValueError("model exceeds manifest size")
                        out.write(block)
            if not matches(temporary, item):
                raise ValueError(f"model checksum failed: {name}")
            temporary.chmod(0o644)
            os.replace(temporary, target)
            print(f"installed and verified {name}", flush=True)
        finally:
            if temporary is not None:
                temporary.unlink(missing_ok=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--manifest", type=Path, default=Path(__file__).resolve().parents[1] / "internal/video/model.lock.json")
    parser.add_argument("--destination", type=Path, required=True)
    args = parser.parse_args()
    install(args.manifest, args.destination)
