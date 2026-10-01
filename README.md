# AWG-SProxy

A lightweight **AmneziaWG 2.0 / 3.1 proxy client** that runs entirely in
userspace. It routes only applications configured to use its SOCKS5 or HTTP
proxy through AmneziaWG, without creating a system-wide VPN interface.

## Fork status and disclaimer

This repository is a fork of the original AWG-SProxy project. The fork adds
and verifies support for:

- AmneziaWG 2.0 configurations;
- AmneziaWG 3.1 configurations;
- the official `github.com/amnezia-vpn/amneziawg-go/v3` backend;
- preshared keys, AWG 3.1 parameters, range keepalives, stricter validation,
  and regression tests.

> **No warranty:** the changes in this fork are the result of vibe coding with
> AI assistance. They have been reviewed and tested against the current
> backend, but may still contain defects, including security or compatibility
> issues. Review the source before relying on it. Use it entirely at your own
> risk. The MIT license provides the software **“AS IS”**, without warranty.

---

## What it does

| Full AmneziaWG client | AWG-SProxy |
|---|---|
| Creates a Windows TUN adapter | No system interface |
| Routes the whole machine | Only apps using the proxy |
| Needs admin / driver install | Runs as a normal user |
| Kill-switch can block other traffic | Rest of the system untouched |

AWG-SProxy uses the official
[amneziawg-go/v3](https://github.com/amnezia-vpn/amneziawg-go) implementation
and its gVisor userspace netstack. Traffic from the proxy listeners goes
through the encrypted AWG tunnel; everything else uses the normal system
connection.

---

## Features

- **SOCKS5** proxy (default port `8600`) — primary, recommended
- **HTTP CONNECT** proxy (default port `8601`)
- Reads native AmneziaWG / WireGuard-style `.conf` files
- Supports one `[Interface]` and exactly one `[Peer]`
- Supports **AWG 2.0**: `Jc`, `Jmin`, `Jmax`, `S1`–`S4`, `H1`–`H4`,
  and `I1`–`I5`
- Supports **AWG 3.1**: `HeaderProtectionKey`, `ContentPaddingAddition`,
  timing ranges, `RandomTrailers`, and `DisableCookies`
- Supports `PresharedKey` and numeric/range `PersistentKeepalive`
- DNS resolved through the tunnel using the DNS servers from your config (same logic as the main AmneziaWG client)
- Strict validation prevents unknown wire-format parameters from being silently ignored
- Binds to `127.0.0.1` by default — not exposed to your LAN
- Optional endpoint override without editing the config file
- No TUN/TAP interface, routing table changes, or root/administrator privileges

---

## Requirements

- **Linux or Windows**
- A working **AmneziaWG / Cloudflare WARP** config file
- A reachable **Peer Endpoint** (`IPv4:port`, `[IPv6]:port`, or hostname)

Building from source requires **Go 1.25+**.

---

## Quick start

### 1. Create your config

Choose the template matching your server:

```bat
REM AmneziaWG 2.0
copy config.awg20.conf.example config.conf

REM AmneziaWG 3.1
copy config.awg31.conf.example config.conf
```

Edit `config.conf` with your keys, address, peer endpoint, allowed IPs, and the
exact AWG parameters supplied by your VPN provider.

> **Never commit or share a real `.conf` file.** It normally contains
> `PrivateKey`, may contain `PresharedKey`, and AWG 3.1 contains
> `HeaderProtectionKey`.

### 2. Run

Double-click `run.bat`, or from a terminal:

```bat
awg-sproxy.exe
```

Expected output:

```text
AWG-SProxy started
  endpoint: vpn.example.net:51820
  dns:      1.1.1.1, 1.0.0.1, ...
  tunnel:   userspace netstack (no system interface)
  socks5:   127.0.0.1:8600
  http:     127.0.0.1:8601
```

Stop with `Ctrl+C`.

## Build from source

Clone your fork/repository and run one of the following commands.

**Windows:**

```bat
build.bat
```

**Linux:**

```bash
go build -o awg-sproxy .
```

**Cross-compile Windows x64 from Linux:**

```bash
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o awg-sproxy.exe .
```

The generated executable is self-contained. Go is not required on the target
machine.

---

## CLI options

| Flag | Default | Description |
|------|---------|-------------|
| `-config` | `config.conf` | Path to AmneziaWG config |
| `-endpoint` | *(from config)* | Override peer endpoint, e.g. `vpn.example.net:51820` |
| `-socks` | `8600` | SOCKS5 listen port (`0` = disable) |
| `-http` | `8601` | HTTP proxy listen port (`0` = disable) |
| `-bind` | `127.0.0.1` | Bind address for proxy listeners |

**Examples:**

```bat
REM Custom ports
awg-sproxy.exe -config config.conf -socks 9000 -http 9001

REM SOCKS5 only
awg-sproxy.exe -config config.conf -http 0

REM Try a different endpoint without editing config
awg-sproxy.exe -config config.conf -endpoint vpn.example.net:51820
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

The parser accepts AmneziaWG / WireGuard-style INI syntax. Section names and
parameter names are case-insensitive, whitespace around `=` is ignored, and
full-line or trailing `#` / `;` comments are supported.

```ini
[Interface]
PrivateKey = <base64 32-byte private key>
Address = 10.8.0.2/32
DNS = 1.1.1.1, 1.0.0.1
MTU = 1280

# AWG 2.0 parameters
Jc = 4
Jmin = 10
Jmax = 50
S1 = 12
S2 = 12
S3 = 12
S4 = 12
H1 = 1
H2 = 2
H3 = 3
H4 = 4
I1 = <r 2><b 0x0102><d>

# AWG 3.1 parameters, only when supplied by the provider
HeaderProtectionKey = <base64 32-byte header protection key>
ContentPaddingAddition = 16-32
RekeyAfterTime = 100-120
RekeyTimeout = 3-7
RejectAfterTime = 150-180
KeepaliveTimeout = 5-15
MaxHandshakeAttempts = 15-20
RandomTrailers = on
DisableCookies = on

[Peer]
PublicKey = <base64 32-byte public key>
PresharedKey = <optional base64 32-byte preshared key>
AllowedIPs = 0.0.0.0/0, ::/0
Endpoint = vpn.example.net:51820
PersistentKeepalive = 25-35
```

Copy every AWG parameter exactly from a known-working provider configuration.
Do not invent values.

### Supported formats

| Parameter | Accepted `.conf` format | Backend UAPI |
|---|---|---|
| Private/Public/Preshared key | Base64, exactly 32 decoded bytes | lowercase hex |
| `HeaderProtectionKey` | Base64, exactly 32 decoded bytes | `header_protection_key=<64 hex chars>` |
| `Jc`, `Jmin`, `Jmax` | decimal uint32 | unchanged |
| `S1`–`S4` | decimal uint16 | unchanged |
| `H1`–`H4` | number or `min-max` | unchanged |
| `I1`–`I5` | AWG obfuscation specification | unchanged |
| AWG 3.1 timing/padding values | number or `min-max` | unchanged |
| `RandomTrailers`, `DisableCookies` | `on/off`, `true/false`, or `1/0` | `1/0` |
| `PersistentKeepalive` | number or `min-max` | unchanged |

`HeaderProtectionKey` requires all `S1`–`S4` values to be at least 12, as
required by the current backend. `H1`–`H4` ranges must not overlap.

### Strict behavior and limitations

- `PrivateKey`, `Address`, `PublicKey`, `Endpoint`, and `AllowedIPs` are required.
- Exactly one `[Interface]` and one `[Peer]` are supported.
- Unknown or duplicate parameters produce an error instead of being ignored.
- Invalid keys, booleans, numeric values, ranges, endpoints, and CIDRs are
  rejected before the tunnel starts.
- `PersistentKeepalive` is not invented when absent.
- Hostname endpoints are resolved once when the client starts.
- `FwMark`, `Table`, `PreUp`, `PostUp`, `PreDown`, `PostDown`, and
  `SaveConfig` are intentionally unsupported by this userspace proxy.

Templates:

- `config.awg20.conf.example` — complete AWG 2.0 example;
- `config.awg31.conf.example` — complete AWG 3.1 example;
- `config.awg20-min.conf.example` — minimal AWG 2.0 template.

---

## Project structure

```text
awg-sproxy/
├── main.go              # Entry point, CLI, lifecycle
├── config.go            # Strict config parser and validation
├── tunnel.go            # AWG netstack and UAPI configuration
├── config_test.go       # AWG 2.0 / 3.1 regression tests
├── proxy/
│   ├── socks5.go        # SOCKS5 server
│   ├── http.go          # HTTP CONNECT proxy
│   ├── dialer.go        # Tunnel dialer
│   └── relay.go         # Connection relay
├── scripts/
│   ├── decode_awg_lnk.py
│   └── test_proxy.py    # Optional local smoke test
├── config.awg20-min.conf.example # Minimal AWG 2.0 template
├── config.awg20.conf.example  # AWG 2.0 template
├── config.awg31.conf.example  # AWG 3.1 template
├── build.bat            # Windows build script
├── run.bat              # Windows run helper
└── README.md
```

---

## Smoke test (optional, source repo only)

If you cloned the full repo and have Python installed, with AWG-SProxy running:

```bat
python scripts\test_proxy.py
```

Run the Go regression suite without a VPN server:

```bash
go test ./...
go vet ./...
go build
```

The tests validate generated UAPI through the real `amneziawg-go/v3`
`IpcSet`/`IpcGet` parser without contacting a VPN server.

---

## Troubleshooting

### `bind: Only one usage of each socket address...`

Another `awg-sproxy` (or another app) is already using port 8600/8601.

```bat
netstat -ano | findstr ":8600"
taskkill /PID <pid> /F
```

Or use different ports: `awg-sproxy.exe -socks 8610 -http 8611`

### Proxy starts but connections fail

- Check that `Endpoint` in your config is reachable from your network.
- Try `-endpoint` with a known-good IP:port.
- Confirm every AWG 2.0/3.1 parameter matches a known-working config exactly.
- Compare the startup AWG UAPI diagnostics. Private keys and PSK are never
  printed; only the first 8 hex characters and length of
  `header_protection_key` are shown.

### Configuration is rejected

- Unknown parameters are rejected intentionally to prevent silent wire-format
  mismatches.
- A configuration with multiple `[Peer]` sections is not supported.
- With `HeaderProtectionKey`, all `S1`–`S4` values must be at least 12.
- `H1`–`H4` ranges must not overlap.

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
  AmneziaWG peer endpoint
```

No Wintun. No routing table changes. No kill-switch.

---

## Credits

- Forked from [moein8668-git/awg-sproxy](https://github.com/moein8668-git/awg-sproxy)
- [amneziawg-go/v3](https://github.com/amnezia-vpn/amneziawg-go) — official AmneziaWG userspace implementation
- Inspired by [wireproxy-awg](https://github.com/artem-russkikh/wireproxy-awg) and similar netstack proxy tools
- AWG 2.0/3.1 compatibility fixes, validation, and tests were developed with
  AI-assisted vibe coding; see the disclaimer at the top of this document

---

## License

MIT — see [LICENSE](LICENSE). The original copyright and permission notice are
retained, and fork modifications are distributed under the same license.
