package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type AWGConfig struct {
	PrivateKey string
	PublicKey  string
	Endpoint   string
	Addresses  []string
	DNS        []string
	MTU        int
	AllowedIPs []string
	Params     map[string]string
}

func loadConfig(path string) (AWGConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return AWGConfig{}, err
	}
	defer file.Close()

	sections := map[string]map[string]string{}
	current := ""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			current = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			if _, ok := sections[current]; !ok {
				sections[current] = map[string]string{}
			}
			continue
		}
		if current == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		sections[current][strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return AWGConfig{}, err
	}

	iface := sections["interface"]
	peer := sections["peer"]
	if iface == nil || peer == nil {
		return AWGConfig{}, fmt.Errorf("config must contain [Interface] and [Peer] sections")
	}

	mtu := 1420
	if value := iface["mtu"]; value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			mtu = parsed
		}
	}

	cfg := AWGConfig{
		PrivateKey: iface["privatekey"],
		PublicKey:  peer["publickey"],
		Endpoint:   peer["endpoint"],
		Addresses:  splitCSV(iface["address"]),
		DNS:        splitCSV(iface["dns"]),
		MTU:        mtu,
		AllowedIPs: splitCSV(peer["allowedips"]),
		Params:     map[string]string{},
	}
	if cfg.PrivateKey == "" {
		return AWGConfig{}, fmt.Errorf("missing Interface PrivateKey")
	}
	if cfg.PublicKey == "" {
		return AWGConfig{}, fmt.Errorf("missing Peer PublicKey")
	}
	if cfg.Endpoint == "" {
		return AWGConfig{}, fmt.Errorf("missing Peer Endpoint")
	}
	if len(cfg.Addresses) == 0 {
		cfg.Addresses = []string{"172.16.0.2/32"}
	}
	if len(cfg.DNS) == 0 {
		cfg.DNS = []string{"1.1.1.1"}
	}
	if len(cfg.AllowedIPs) == 0 {
		cfg.AllowedIPs = []string{"0.0.0.0/0", "::/0"}
	}

	for _, key := range []string{"jc", "jmin", "jmax", "s1", "s2", "s3", "s4", "h1", "h2", "h3", "h4", "i1", "i2", "i3", "i4", "i5"} {
		if value := iface[key]; value != "" {
			cfg.Params[key] = value
		}
	}

	return cfg, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
