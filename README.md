# AWG-SProxy

A lightweight **AmneziaWG proxy** that runs entirely in userspace. Route only the apps you choose through Cloudflare WARP / AmneziaWG — without installing a system-wide VPN tunnel.

> **Note:** This project was **vibe-coded** with AI assistance (Cursor). I did not write the code myself — I designed the idea and iterated on it with AI. Use at your own discretion, review the code if you care about security, and report issues if something breaks.

---

## What it does

| Full AmneziaWG client | AWG-SProxy |
|---|---|
| Creates a Windows TUN adapter | No system interface |
| Routes the whole machine | Only apps using the proxy |
| Needs admin / driver install | Runs as a normal user |
| Kill-switch can block other traffic | Rest of the system untouched |

AWG-SProxy uses [amneziawg-go](https://github.com/amnezia-vpn/amneziawg-go) **netstack** (gVisor userspace network stack). Traffic from the proxy listeners goes through the encrypted AWG tunnel; everything else uses your normal connection.

---

## Features

- **SOCKS5** proxy (default port `8600`) — primary, recommended
- **HTTP CONNECT** proxy (default port `8601`)
- Reads standard AmneziaWG / WARP `config.conf` (Interface + Peer)
- DNS resolved through the tunnel using the DNS servers from your config (same logic as the main AmneziaWG client)
- Supports AWG obfuscation parameters (`Jc`, `Jmin`, `Jmax`, `H1`–`H4`, `I1`, etc.)
- Binds to `127.0.0.1` by default — not exposed to your LAN
- Optional endpoint override without editing the config file

---

## Requirements

- **Go 1.24+** (to build from source)
- A working **AmneziaWG / Cloudflare WARP** config file
- A reachable **Peer Endpoint** (IP:port)

---

## Quick start

### 1. Get the config

Copy the example and add your real keys:

```bat
copy config.conf.example config.conf
```

Edit `config.conf` with your `PrivateKey`, `Address`, AWG noise params, and `Endpoint`.

### 2. Build

**Windows:**

```bat
build.bat
```

**Any OS:**

```bash
go build -o awg-sproxy .
```

### 3. Run

```bat
awg-sproxy.exe
```

Or on Windows with the helper script (checks that `config.conf` exists):

```bat
run.bat
```

Expected output:

```text
AWG-SProxy started
  endpoint: 188.114.97.6:7281
  dns:      1.1.1.1, 1.0.0.1, ...
  tunnel:   userspace netstack (no system interface)
  socks5:   127.0.0.1:8600
  http:     127.0.0.1:8601
```

Stop with `Ctrl+C`.

---

## CLI options

| Flag | Default | Description |
|------|---------|-------------|
| `-config` | `config.conf` | Path to AmneziaWG config |
| `-endpoint` | *(from config)* | Override peer endpoint, e.g. `188.114.97.6:7281` |
| `-socks` | `8600` | SOCKS5 listen port (`0` = disable) |
| `-http` | `8601` | HTTP proxy listen port (`0` = disable) |
| `-bind` | `127.0.0.1` | Bind address for proxy listeners |

**Examples:**

```bat
REM Custom ports
awg-sproxy.exe -config config.conf --socks 9000 --http 9001

REM SOCKS5 only
awg-sproxy.exe -config config.conf --http 0

REM Try a different endpoint without editing config
awg-sproxy.exe -config config.conf -endpoint 8.6.112.208:7281
```

---

## Using the proxy in apps

### SOCKS5 (recommended)

```text
Host: 127.0.0.1
Port: 8600
Type: SOCKS5
```

Use **`socks5h://`** (remote DNS) when the app supports it, so hostnames are resolved inside the tunnel:

```bat
curl -x socks5h://127.0.0.1:8600 https://1.1.1.1/cdn-cgi/trace
```

### HTTP proxy

```text
http://127.0.0.1:8601
```

```bat
curl -x http://127.0.0.1:8601 https://1.1.1.1/cdn-cgi/trace
```

### Browser

Set **manual proxy** to `127.0.0.1:8601` (HTTP) or use a SOCKS5 extension pointing at `127.0.0.1:8600`.

Only tabs/apps using the proxy go through WARP. Everything else stays on your normal connection.

---

## Config file format

Standard AmneziaWG / WireGuard INI format:

```ini
[Interface]
PrivateKey = ...
Address = 172.16.0.2/32, 2606:4700:110:.../128
DNS = 1.1.1.1, 1.0.0.1, 2606:4700:4700::1111, 2606:4700:4700::1001
MTU = 1280
Jc = 3
Jmin = 1
Jmax = 3
H1 = 1
H2 = 2
H3 = 3
H4 = 4

[Peer]
PublicKey = bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=
AllowedIPs = 0.0.0.0/0, ::/0
Endpoint = 188.114.97.6:7281
```

See `config.conf.example` for a template.

---

## Project structure

```text
awg-sproxy/
├── main.go              # Entry point, CLI, lifecycle
├── config.go            # Config parser
├── tunnel.go            # AWG netstack tunnel setup
├── proxy/
│   ├── socks5.go        # SOCKS5 server
│   ├── http.go          # HTTP CONNECT proxy
│   ├── dialer.go        # Tunnel dialer
│   └── relay.go         # Connection relay
├── scripts/
│   └── test_proxy.py    # Optional local smoke test
├── config.conf.example  # Template (safe to commit)
├── build.bat            # Windows build script
├── run.bat              # Windows run helper
└── README.md
```

---

## Smoke test (optional)

With AWG-SProxy running and a working endpoint:

```bat
python scripts\test_proxy.py
```

---

## Troubleshooting

### `bind: Only one usage of each socket address...`

Another `awg-sproxy` (or another app) is already using port 8600/8601.

```bat
netstat -ano | findstr ":8600"
taskkill /PID <pid> /F
```

Or use different ports: `awg-sproxy.exe --socks 8610 --http 8611`

### Proxy starts but connections fail

- Check that `Endpoint` in your config is reachable from your network.
- Try `-endpoint` with a known-good IP:port.
- Confirm AWG noise params (`Jc`, `H1`, etc.) match your working config exactly.

### SOCKS5 handshake works but sites time out

- Use `socks5h://` so DNS goes through the tunnel.
- Verify DNS servers in `[Interface]` are correct.

---

## How it works

```text
  App (browser, curl, …)
        │
        ▼
  SOCKS5 :8600  or  HTTP :8601
        │
        ▼
  netstack dial (DNS via config DNS servers)
        │
        ▼
  AmneziaWG device (userspace, encrypted)
        │
        ▼
  WARP peer endpoint
```

No Wintun. No routing table changes. No kill-switch.

---

## Credits

- [amneziawg-go](https://github.com/amnezia-vpn/amneziawg-go) — AmneziaWG userspace implementation
- Inspired by [wireproxy-awg](https://github.com/artem-russkikh/wireproxy-awg) and similar netstack proxy tools

---

## License

MIT — see [LICENSE](LICENSE).
