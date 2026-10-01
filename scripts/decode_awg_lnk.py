#!/usr/bin/env python3

import sys
import base64
import zlib
import json


def decode_qcompress(data: bytes) -> bytes:
    # Qt qCompress:
    # first 4 bytes = uncompressed length (big endian)
    if len(data) > 4:
        try:
            return zlib.decompress(data[4:])
        except zlib.error:
            pass

    # fallback: maybe payload wasn't compressed
    return data


if len(sys.argv) != 2:
    print(f"Usage: {sys.argv[0]} 'vpn://...'", file=sys.stderr)
    sys.exit(1)

s = sys.argv[1].strip()

if s.startswith("vpn://"):
    s = s[6:]

# Base64URL padding
s += "=" * ((4 - len(s) % 4) % 4)

raw = base64.urlsafe_b64decode(s)
decoded = decode_qcompress(raw)

text = decoded.decode("utf-8")

print("===== DECODED =====")
print(text)

try:
    obj = json.loads(text)
except json.JSONDecodeError:
    sys.exit(0)

print("\n===== POSSIBLE AWG CONFIGS =====")

for i, container in enumerate(obj.get("containers", [])):
    for key in ("awg", "amnezia-awg", "amnezia-awg2", "wireguard"):
        entry = container.get(key)
        if not isinstance(entry, dict):
            continue

        last = entry.get("last_config")

        if isinstance(last, str):
            try:
                last = json.loads(last)
            except json.JSONDecodeError:
                continue

        if not isinstance(last, dict):
            continue

        cfg = last.get("config")

        if cfg:
            print(f"\n--- containers[{i}].{key}.last_config.config ---")
            print(cfg)

        mtu = last.get("mtu")
        if mtu:
            print(f"MTU = {mtu}")
