package main

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/netip"
	"strings"

	"github.com/amnezia-vpn/amneziawg-go/conn"
	"github.com/amnezia-vpn/amneziawg-go/device"
	"github.com/amnezia-vpn/amneziawg-go/tun/netstack"
)

type Tunnel struct {
	Device *device.Device
	Net    *netstack.Net
}

func startTunnel(cfg AWGConfig) (*Tunnel, error) {
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

	uapi, err := buildUAPI(cfg)
	if err != nil {
		dev.Close()
		return nil, err
	}
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
	privateKey, err := base64KeyToHex(cfg.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("invalid private key: %w", err)
	}
	publicKey, err := base64KeyToHex(cfg.PublicKey)
	if err != nil {
		return "", fmt.Errorf("invalid public key: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "private_key=%s\n", privateKey)
	for _, key := range []string{"jc", "jmin", "jmax", "s1", "s2", "s3", "s4", "h1", "h2", "h3", "h4", "i1", "i2", "i3", "i4", "i5"} {
		if value := cfg.Params[key]; value != "" {
			fmt.Fprintf(&b, "%s=%s\n", key, value)
		}
	}
	fmt.Fprintf(&b, "public_key=%s\n", publicKey)
	fmt.Fprintf(&b, "endpoint=%s\n", cfg.Endpoint)
	fmt.Fprintf(&b, "persistent_keepalive_interval=5\n")
	fmt.Fprintf(&b, "replace_allowed_ips=true\n")
	for _, allowed := range cfg.AllowedIPs {
		fmt.Fprintf(&b, "allowed_ip=%s\n", allowed)
	}
	b.WriteByte('\n')

	return b.String(), nil
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
