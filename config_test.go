package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"log"
	"net/netip"
	"strings"
	"testing"

	"github.com/amnezia-vpn/amneziawg-go/v3/conn"
	"github.com/amnezia-vpn/amneziawg-go/v3/device"
	"github.com/amnezia-vpn/amneziawg-go/v3/tun/netstack"
)

func testKey(value byte) string {
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{value}, 32))
}

func testConfig(interfaceExtra, peerExtra string) string {
	return fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = 10.8.0.2/32, fd00::2/128
DNS = 1.1.1.1, 2606:4700:4700::1111
MTU = 1280
%s

[Peer]
PublicKey = %s
PresharedKey = %s
AllowedIPs = 0.0.0.0/0, ::/0
Endpoint = 192.0.2.1:51820
%s
`, testKey(1), interfaceExtra, testKey(2), testKey(3), peerExtra)
}

func mustParseConfig(t *testing.T, text string) AWGConfig {
	t.Helper()
	cfg, err := parseConfig(strings.NewReader(text))
	if err != nil {
		t.Fatalf("parseConfig() error: %v", err)
	}
	return cfg
}

func mustBuildUAPI(t *testing.T, cfg AWGConfig) string {
	t.Helper()
	uapi, err := buildUAPI(cfg)
	if err != nil {
		t.Fatalf("buildUAPI() error: %v", err)
	}
	return uapi
}

func requireUAPIContains(t *testing.T, uapi string, lines ...string) {
	t.Helper()
	for _, line := range lines {
		if !strings.Contains(uapi, line+"\n") {
			t.Errorf("UAPI does not contain %q:\n%s", line, uapi)
		}
	}
}

func requireBackendAccepts(t *testing.T, uapi string) {
	t.Helper()
	tunDevice, _, err := netstack.CreateNetTUN(
		[]netip.Addr{netip.MustParseAddr("10.8.0.2")},
		[]netip.Addr{netip.MustParseAddr("1.1.1.1")},
		1280,
	)
	if err != nil {
		t.Fatalf("CreateNetTUN() error: %v", err)
	}
	dev := device.NewDevice(
		tunDevice,
		conn.NewDefaultBind(),
		device.NewLogger(device.LogLevelSilent, ""),
	)
	defer dev.Close()

	if err := dev.IpcSet(uapi); err != nil {
		t.Fatalf("backend rejected generated UAPI: %v\n%s", err, uapi)
	}
	state, err := dev.IpcGet()
	if err != nil {
		t.Fatalf("backend IpcGet() error: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(uapi), "\n") {
		if strings.HasPrefix(line, "private_key=") || line == "replace_allowed_ips=true" {
			continue
		}
		if !strings.Contains(state, line+"\n") {
			t.Errorf("backend state does not contain applied UAPI line %q:\n%s", line, state)
		}
	}
}

func TestAWG20ConfigParsing(t *testing.T) {
	config := fmt.Sprintf(`# full-line comment
[iNtErFaCe]
 privateKEY = %s # inline comment
 ADDRESS = 10.8.0.2/32, fd00::2/128
 dns = 1.1.1.1
 Jc = 4
 Jmin = 10
 Jmax = 50
 S1 = 12
 S2 = 13
 S3 = 14
 S4 = 15
 H1 = 1
 H2 = 2-3
 H3 = 4
 H4 = 5
 I1 = <r 2><b 0x0102><d> ; inline comment
 ; another full-line comment

[pEeR]
 PUBLICkey = %s
 PresharedKEY = %s
 allowedIPS = 0.0.0.0/0, ::/0
 endpoint = [2001:db8::1]:51820 # inline comment
 PersistentKeepalive = 25
`, testKey(1), testKey(2), testKey(3))

	cfg := mustParseConfig(t, config)
	if cfg.PersistentKeepalive != "25" {
		t.Errorf("PersistentKeepalive = %q, want 25", cfg.PersistentKeepalive)
	}
	if cfg.Endpoint != "[2001:db8::1]:51820" {
		t.Errorf("Endpoint = %q", cfg.Endpoint)
	}
	if got := cfg.Params["h2"]; got != "2-3" {
		t.Errorf("H2 = %q, want 2-3", got)
	}
	if got := cfg.Params["i1"]; got != "<r 2><b 0x0102><d>" {
		t.Errorf("I1 changed: %q", got)
	}
	if len(cfg.AllowedIPs) != 2 || cfg.AllowedIPs[1] != "::/0" {
		t.Errorf("AllowedIPs = %#v", cfg.AllowedIPs)
	}
}

func TestAWG20GeneratedUAPI(t *testing.T) {
	cfg := mustParseConfig(t, testConfig(`
ListenPort = 51821
Jc = 4
Jmin = 10
Jmax = 50
S1 = 12
S2 = 13
S3 = 14
S4 = 15
H1 = 1
H2 = 2
H3 = 3
H4 = 4
I1 = <r 2><b 0x0102><d>`, "PersistentKeepalive = 25"))

	uapi := mustBuildUAPI(t, cfg)
	requireUAPIContains(t, uapi,
		"private_key="+strings.Repeat("01", 32),
		"listen_port=51821",
		"jc=4", "jmin=10", "jmax=50",
		"s1=12", "s2=13", "s3=14", "s4=15",
		"h1=1", "h2=2", "h3=3", "h4=4",
		"i1=<r 2><b 0x0102><d>",
		"public_key="+strings.Repeat("02", 32),
		"preshared_key="+strings.Repeat("03", 32),
		"endpoint=192.0.2.1:51820",
		"persistent_keepalive_interval=25",
		"allowed_ip=0.0.0.0/0",
		"allowed_ip=::/0",
	)
	for _, key := range []string{
		"header_protection_key=", "content_padding_addition=",
		"rekey_after_time=", "random_trailers=", "disable_cookies=",
	} {
		if strings.Contains(uapi, key) {
			t.Errorf("AWG 2.0 UAPI unexpectedly contains %q", key)
		}
	}
	requireBackendAccepts(t, uapi)
}

func TestAbsentPersistentKeepaliveIsNotInvented(t *testing.T) {
	cfg := mustParseConfig(t, testConfig("", ""))
	uapi := mustBuildUAPI(t, cfg)
	if strings.Contains(uapi, "persistent_keepalive_interval=") {
		t.Fatalf("UAPI contains an invented PersistentKeepalive:\n%s", uapi)
	}
}

func awg31InterfaceParameters() string {
	return fmt.Sprintf(`
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
I2 = <t>
I3 = <rd 3>
I4 = <rc 4>
I5 = <b 0xaabb>
HeaderProtectionKey = %s
ContentPaddingAddition = 16-32
RekeyAfterTime = 100-120
RekeyTimeout = 3-7
RejectAfterTime = 150-180
KeepaliveTimeout = 5-15
MaxHandshakeAttempts = 15-20
RandomTrailers = on
DisableCookies = false`, testKey(4))
}

func TestAWG31ConfigParsing(t *testing.T) {
	cfg := mustParseConfig(t, testConfig(
		awg31InterfaceParameters(),
		"PersistentKeepalive = 25-35",
	))

	want := map[string]string{
		"headerprotectionkey":    testKey(4),
		"contentpaddingaddition": "16-32",
		"rekeyaftertime":         "100-120",
		"rekeytimeout":           "3-7",
		"rejectaftertime":        "150-180",
		"keepalivetimeout":       "5-15",
		"maxhandshakeattempts":   "15-20",
		"randomtrailers":         "on",
		"disablecookies":         "false",
	}
	for key, expected := range want {
		if got := cfg.Params[key]; got != expected {
			t.Errorf("%s = %q, want %q", key, got, expected)
		}
	}
	if cfg.PersistentKeepalive != "25-35" {
		t.Errorf("PersistentKeepalive = %q", cfg.PersistentKeepalive)
	}
}

func TestAWG31GeneratedUAPI(t *testing.T) {
	cfg := mustParseConfig(t, testConfig(
		awg31InterfaceParameters(),
		"PersistentKeepalive = 25-35",
	))
	uapi := mustBuildUAPI(t, cfg)

	requireUAPIContains(t, uapi,
		"header_protection_key="+strings.Repeat("04", 32),
		"content_padding_addition=16-32",
		"rekey_after_time=100-120",
		"rekey_timeout=3-7",
		"reject_after_time=150-180",
		"keepalive_timeout=5-15",
		"max_handshake_attempts=15-20",
		"random_trailers=1",
		"disable_cookies=0",
		"persistent_keepalive_interval=25-35",
	)
	for i, expected := range []string{
		"<r 2><b 0x0102><d>", "<t>", "<rd 3>", "<rc 4>", "<b 0xaabb>",
	} {
		requireUAPIContains(t, uapi, fmt.Sprintf("i%d=%s", i+1, expected))
	}
	requireBackendAccepts(t, uapi)
}

func TestHeaderProtectionKeyValidation(t *testing.T) {
	t.Run("base64 to lowercase hex", func(t *testing.T) {
		cfg := mustParseConfig(t, testConfig(
			"S1 = 12\nS2 = 12\nS3 = 12\nS4 = 12\nHeaderProtectionKey = "+testKey(0xab),
			"",
		))
		uapi := mustBuildUAPI(t, cfg)
		requireUAPIContains(t, uapi, "header_protection_key="+strings.Repeat("ab", 32))
	})

	for name, value := range map[string]string{
		"invalid base64": "not-base64!",
		"wrong length":   base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 31)),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseConfig(strings.NewReader(testConfig(
				"HeaderProtectionKey = "+value,
				"",
			)))
			if err == nil || !strings.Contains(err.Error(), "HeaderProtectionKey") {
				t.Fatalf("error = %v, want HeaderProtectionKey validation error", err)
			}
		})
	}
}

func TestBackendCrossFieldValidation(t *testing.T) {
	t.Run("header protection requires nonce-sized paddings", func(t *testing.T) {
		_, err := parseConfig(strings.NewReader(testConfig(
			"HeaderProtectionKey = "+testKey(4),
			"",
		)))
		if err == nil || !strings.Contains(err.Error(), "must be at least 12") {
			t.Fatalf("error = %v, want header padding validation error", err)
		}
	})

	t.Run("header ranges must not overlap", func(t *testing.T) {
		_, err := parseConfig(strings.NewReader(testConfig(
			"H1 = 1-2\nH2 = 2",
			"",
		)))
		if err == nil || !strings.Contains(err.Error(), "H1 and H2 overlap") {
			t.Fatalf("error = %v, want overlapping header validation error", err)
		}
	})

	t.Run("preshared key length", func(t *testing.T) {
		config := strings.Replace(
			testConfig("", ""),
			"PresharedKey = "+testKey(3),
			"PresharedKey = "+base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, 31)),
			1,
		)
		_, err := parseConfig(strings.NewReader(config))
		if err == nil || !strings.Contains(err.Error(), "PresharedKey") {
			t.Fatalf("error = %v, want PresharedKey validation error", err)
		}
	})
}

func TestAWGBooleanValues(t *testing.T) {
	tests := map[string]string{
		"on": "1", "ON": "1", "true": "1", "TRUE": "1", "1": "1",
		"off": "0", "OFF": "0", "false": "0", "FALSE": "0", "0": "0",
	}
	for input, expected := range tests {
		t.Run(input, func(t *testing.T) {
			got, err := awgBoolToUAPI(input)
			if err != nil || got != expected {
				t.Fatalf("awgBoolToUAPI(%q) = %q, %v; want %q", input, got, err, expected)
			}
		})
	}
	if _, err := awgBoolToUAPI("yes"); err == nil {
		t.Fatal("unknown boolean value was accepted")
	}
}

func TestRangeValidation(t *testing.T) {
	for _, value := range []string{"0", "25", "25-35", "4294967295"} {
		if err := validateUintRange(value); err != nil {
			t.Errorf("valid range %q rejected: %v", value, err)
		}
	}
	for _, value := range []string{"", "-1", "35-25", "1-2-3", "4294967296"} {
		if err := validateUintRange(value); err == nil {
			t.Errorf("invalid range %q accepted", value)
		}
	}
}

func TestIParametersPreserved(t *testing.T) {
	values := []string{
		"<r 2><b 0x010203><d>",
		"<t><b 0xaabb>",
		"<rd 12>",
		"<rc 7>",
		"<ds example>",
	}
	var lines strings.Builder
	for i, value := range values {
		fmt.Fprintf(&lines, "I%d = %s\n", i+1, value)
	}
	cfg := mustParseConfig(t, testConfig(lines.String(), ""))
	uapi := mustBuildUAPI(t, cfg)
	for i, expected := range values {
		if got := cfg.Params[fmt.Sprintf("i%d", i+1)]; got != expected {
			t.Errorf("I%d parsed as %q, want %q", i+1, got, expected)
		}
		requireUAPIContains(t, uapi, fmt.Sprintf("i%d=%s", i+1, expected))
	}
}

func TestUnknownParameterRejected(t *testing.T) {
	_, err := parseConfig(strings.NewReader(testConfig(
		"FutureWireFormat = enabled",
		"",
	)))
	if err == nil || !strings.Contains(err.Error(), `unknown parameter "FutureWireFormat"`) {
		t.Fatalf("error = %v, want unknown parameter error", err)
	}
}

func TestMultiplePeersRejected(t *testing.T) {
	config := testConfig("", "") + fmt.Sprintf(`
[Peer]
PublicKey = %s
AllowedIPs = 10.0.0.0/8
Endpoint = 192.0.2.2:51820
`, testKey(5))
	_, err := parseConfig(strings.NewReader(config))
	if err == nil || !strings.Contains(err.Error(), "multiple [peer] sections") {
		t.Fatalf("error = %v, want multiple peer error", err)
	}
}

func TestPersistentKeepaliveFormats(t *testing.T) {
	for _, value := range []string{"25", "25-35"} {
		t.Run(value, func(t *testing.T) {
			cfg := mustParseConfig(t, testConfig(
				"",
				"PersistentKeepalive = "+value,
			))
			uapi := mustBuildUAPI(t, cfg)
			requireUAPIContains(t, uapi, "persistent_keepalive_interval="+value)
			requireBackendAccepts(t, uapi)
		})
	}

	_, err := parseConfig(strings.NewReader(testConfig(
		"",
		"PersistentKeepalive = invalid",
	)))
	if err == nil || !strings.Contains(err.Error(), "PersistentKeepalive") {
		t.Fatalf("error = %v, want PersistentKeepalive validation error", err)
	}
}

func TestEndpointFormats(t *testing.T) {
	for _, endpoint := range []string{
		"192.0.2.1:51820",
		"[2001:db8::1]:51820",
		"localhost:51820",
	} {
		t.Run(endpoint, func(t *testing.T) {
			resolved, err := resolveEndpoint(endpoint)
			if err != nil {
				t.Fatalf("resolveEndpoint(%q): %v", endpoint, err)
			}
			if _, err := netip.ParseAddrPort(resolved); err != nil {
				t.Fatalf("resolved endpoint %q is not numeric: %v", resolved, err)
			}
		})
	}
}

func TestSensitiveAWGLogRedactsHeaderKey(t *testing.T) {
	headerKey := strings.Repeat("ab", 32)
	var output bytes.Buffer
	oldWriter := log.Writer()
	oldFlags := log.Flags()
	oldPrefix := log.Prefix()
	log.SetOutput(&output)
	log.SetFlags(0)
	log.SetPrefix("")
	defer func() {
		log.SetOutput(oldWriter)
		log.SetFlags(oldFlags)
		log.SetPrefix(oldPrefix)
	}()

	logAWGUAPIParams("private_key=" + strings.Repeat("01", 32) +
		"\npreshared_key=" + strings.Repeat("02", 32) +
		"\nheader_protection_key=" + headerKey + "\n")

	logged := output.String()
	if strings.Contains(logged, headerKey) {
		t.Fatal("full HeaderProtectionKey was logged")
	}
	if strings.Contains(logged, strings.Repeat("01", 32)) ||
		strings.Contains(logged, strings.Repeat("02", 32)) {
		t.Fatal("private key or preshared key was logged")
	}
	if !strings.Contains(logged, "header_protection_key=abababab... (hex length=64)") {
		t.Fatalf("redacted HeaderProtectionKey preview missing:\n%s", logged)
	}
}
