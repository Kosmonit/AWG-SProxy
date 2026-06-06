#!/usr/bin/env python3
"""Quick local smoke test for AWG-SProxy SOCKS5/HTTP listeners."""

import socket
import sys
import urllib.request

SOCKS_HOST = "127.0.0.1"
SOCKS_PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 8600
HTTP_PORT = int(sys.argv[2]) if len(sys.argv) > 2 else 8601
TEST_URL = "http://www.gstatic.com/generate_204"


def test_socks5_connect():
    host = "www.gstatic.com"
    port = 80
    s = socket.create_connection((SOCKS_HOST, SOCKS_PORT), timeout=10)
    s.settimeout(30)
    s.sendall(b"\x05\x01\x00")
    resp = s.recv(2)
    if resp != b"\x05\x00":
        raise RuntimeError(f"socks greet failed: {resp!r}")

    req = bytearray([0x05, 0x01, 0x00, 0x03, len(host)])
    req.extend(host.encode())
    req.extend(port.to_bytes(2, "big"))
    s.sendall(req)
    resp = s.recv(10)
    if len(resp) < 2 or resp[1] != 0:
        raise RuntimeError(f"socks connect failed: {resp!r}")

    s.sendall(b"HEAD /generate_204 HTTP/1.1\r\nHost: www.gstatic.com\r\n\r\n")
    data = s.recv(256)
    s.close()
    if b"204" not in data and b"200" not in data:
        raise RuntimeError(f"unexpected http response: {data[:120]!r}")
    return True


def test_http_proxy():
    proxy = urllib.request.ProxyHandler({"http": f"http://{SOCKS_HOST}:{HTTP_PORT}"})
    opener = urllib.request.build_opener(proxy)
    req = urllib.request.Request(TEST_URL, method="HEAD")
    with opener.open(req, timeout=20) as resp:
        return resp.status == 204


def main():
    print(f"Testing SOCKS5 {SOCKS_HOST}:{SOCKS_PORT} ...")
    try:
        test_socks5_connect()
        print("  SOCKS5: OK")
    except Exception as e:
        print(f"  SOCKS5: FAILED ({e})")
        return 1

    print(f"Testing HTTP {SOCKS_HOST}:{HTTP_PORT} ...")
    try:
        if test_http_proxy():
            print("  HTTP: OK")
        else:
            print("  HTTP: FAILED (bad status)")
            return 1
    except Exception as e:
        print(f"  HTTP: FAILED ({e})")
        return 1

    print("All proxy tests passed.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
