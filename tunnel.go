package main

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/netip"
	"strings"

	"github.com/amnezia-vpn/amneziawg-go/v3/conn"
	"github.com/amnezia-vpn/amneziawg-go/v3/device"
	"github.com/amnezia-vpn/amneziawg-go/v3/tun/netstack"
)

type Tunnel struct {
	Device *device.Device
	Net    *netstack.Net
}

func startTunnel(cfg AWGConfig) (*Tunnel, error) {
	uapi, err := buildUAPI(cfg)
	if err != nil {
		return nil, err
	}

	localAddrs, err := parseAddrList(cfg.Addresses)
	if err != nil {
		return nil, err
	}
	dnsAddrs, err := parseAddrList(cfg.DNS)
	if err != nil {
		return nil, err
	}

	tdev, tnet, err := netstack.CreateNetTUN(localAddrs, dnsAddrs, cfg.MTU)
	if err != nil {
		return nil, fmt.Errorf("create netstack: %w", err)
	}

	dev := device.NewDevice(tdev, conn.NewDefaultBind(), device.NewLogger(device.LogLevelSilent, ""))

	logAWGUAPIParams(uapi)
	if err := dev.IpcSet(uapi); err != nil {
		dev.Close()
		return nil, fmt.Errorf("configure device: %w", err)
	}
	if err := dev.Up(); err != nil {
		dev.Close()
		return nil, fmt.Errorf("bring device up: %w", err)
	}

	return &Tunnel{Device: dev, Net: tnet}, nil
}

func (t *Tunnel) Close() {
	if t.Device != nil {
		t.Device.Close()
	}
}

func buildUAPI(cfg AWGConfig) (string, error) {
	if err := validateConfig(cfg); err != nil {
		return "", err
	}

	privateKey, err := base64KeyToHex(cfg.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("invalid private key: %w", err)
	}
	publicKey, err := base64KeyToHex(cfg.PublicKey)
	if err != nil {
		return "", fmt.Errorf("invalid public key: %w", err)
	}

	// FIX
	var presharedKey string
	if strings.TrimSpace(cfg.PresharedKey) != "" {
		presharedKey, err = base64KeyToHex(cfg.PresharedKey)
		if err != nil {
			return "", fmt.Errorf("invalid preshared key: %w", err)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "private_key=%s\n", privateKey)
	if cfg.ListenPort != "" {
		fmt.Fprintf(&b, "listen_port=%s\n", cfg.ListenPort)
	}
	for _, key := range []string{"jc", "jmin", "jmax", "s1", "s2", "s3", "s4", "h1", "h2", "h3", "h4", "i1", "i2", "i3", "i4", "i5"} {
		if value := cfg.Params[key]; value != "" {
			fmt.Fprintf(&b, "%s=%s\n", key, value)
		}
	}

	if value := cfg.Params["headerprotectionkey"]; value != "" {
		headerProtectionKey, err := base64KeyToHex(value)
		if err != nil {
			return "", fmt.Errorf("invalid header protection key: %w", err)
		}
		fmt.Fprintf(&b, "header_protection_key=%s\n", headerProtectionKey)
	}

	for _, param := range []struct {
		configKey string
		uapiKey   string
	}{
		{"contentpaddingaddition", "content_padding_addition"},
		{"rekeyaftertime", "rekey_after_time"},
		{"rekeytimeout", "rekey_timeout"},
		{"rejectaftertime", "reject_after_time"},
		{"keepalivetimeout", "keepalive_timeout"},
		{"maxhandshakeattempts", "max_handshake_attempts"},
	} {
		if value := cfg.Params[param.configKey]; value != "" {
			fmt.Fprintf(&b, "%s=%s\n", param.uapiKey, value)
		}
	}

	for _, param := range []struct {
		configKey string
		uapiKey   string
	}{
		{"randomtrailers", "random_trailers"},
		{"disablecookies", "disable_cookies"},
	} {
		if value := cfg.Params[param.configKey]; value != "" {
			uapiValue, err := awgBoolToUAPI(value)
			if err != nil {
				return "", fmt.Errorf("invalid %s: %w", param.configKey, err)
			}
			fmt.Fprintf(&b, "%s=%s\n", param.uapiKey, uapiValue)
		}
	}

	fmt.Fprintf(&b, "public_key=%s\n", publicKey)

	if presharedKey != "" {
		fmt.Fprintf(&b, "preshared_key=%s\n", presharedKey)
	}

	endpoint, err := resolveEndpoint(cfg.Endpoint)
	if err != nil {
		return "", fmt.Errorf("resolve endpoint: %w", err)
	}
	fmt.Fprintf(&b, "endpoint=%s\n", endpoint)
	if cfg.PersistentKeepalive != "" {
		fmt.Fprintf(&b, "persistent_keepalive_interval=%s\n", cfg.PersistentKeepalive)
	}
	fmt.Fprintf(&b, "replace_allowed_ips=true\n")
	for _, allowed := range cfg.AllowedIPs {
		fmt.Fprintf(&b, "allowed_ip=%s\n", allowed)
	}
	b.WriteByte('\n')

	return b.String(), nil
}

func awgBoolToUAPI(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "on", "true", "1":
		return "1", nil
	case "off", "false", "0":
		return "0", nil
	default:
		return "", fmt.Errorf("expected on/off, true/false, or 1/0, got %q", value)
	}
}

func resolveEndpoint(value string) (string, error) {
	if endpoint, err := netip.ParseAddrPort(value); err == nil {
		return endpoint.String(), nil
	}

	endpoint, err := net.ResolveUDPAddr("udp", value)
	if err != nil {
		return "", err
	}
	if endpoint.IP == nil {
		return "", fmt.Errorf("endpoint host did not resolve to an IP address")
	}
	ip := endpoint.IP
	if ipv4 := ip.To4(); ipv4 != nil {
		ip = ipv4
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return "", fmt.Errorf("endpoint resolved to an invalid IP address")
	}
	if endpoint.Zone != "" {
		addr = addr.WithZone(endpoint.Zone)
	}
	return netip.AddrPortFrom(addr, uint16(endpoint.Port)).String(), nil
}

func logAWGUAPIParams(uapi string) {
	values := make(map[string]string)
	for _, line := range strings.Split(uapi, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			values[key] = value
		}
	}

	log.Print("AWG UAPI parameters:")
	for _, key := range []string{
		"jc", "jmin", "jmax",
		"s1", "s2", "s3", "s4",
		"h1", "h2", "h3", "h4",
		"i1", "i2", "i3", "i4", "i5",
		"header_protection_key",
		"content_padding_addition",
		"rekey_after_time",
		"rekey_timeout",
		"reject_after_time",
		"keepalive_timeout",
		"max_handshake_attempts",
		"random_trailers",
		"disable_cookies",
	} {
		value, ok := values[key]
		if !ok {
			log.Printf("  %s=<unset>", key)
			continue
		}
		if key == "header_protection_key" {
			preview := value
			if len(preview) > 8 {
				preview = preview[:8]
			}
			log.Printf("  %s=%s... (hex length=%d)", key, preview, len(value))
			continue
		}
		log.Printf("  %s=%s", key, value)
	}
}

func base64KeyToHex(value string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return "", err
	}
	if len(decoded) != 32 {
		return "", fmt.Errorf("expected 32 bytes, got %d", len(decoded))
	}
	return hex.EncodeToString(decoded), nil
}

func parseAddrList(values []string) ([]netip.Addr, error) {
	addrs := make([]netip.Addr, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if prefix, err := netip.ParsePrefix(value); err == nil {
			addrs = append(addrs, prefix.Addr())
			continue
		}
		addr, err := netip.ParseAddr(value)
		if err != nil {
			return nil, fmt.Errorf("invalid address %q: %w", value, err)
		}
		addrs = append(addrs, addr)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("no valid addresses")
	}
	return addrs, nil
}
