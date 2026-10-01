package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/amnezia-vpn/amneziawg-go/v3/device"
)

//type AWGConfig struct {
//	PrivateKey string
//	PublicKey  string
//	Endpoint   string
//	Addresses  []string
//	DNS        []string
//	MTU        int
//	AllowedIPs []string
//	Params     map[string]string
//}

type AWGConfig struct {
	PrivateKey          string
	ListenPort          string
	PublicKey           string
	PresharedKey        string
	Endpoint            string
	PersistentKeepalive string
	Addresses           []string
	DNS                 []string
	MTU                 int
	AllowedIPs          []string
	Params              map[string]string
}

func loadConfig(path string) (AWGConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return AWGConfig{}, err
	}
	defer file.Close()

	return parseConfig(file)
}

var knownConfigKeys = map[string]map[string]struct{}{
	"interface": {
		"privatekey": {}, "listenport": {}, "address": {}, "dns": {}, "mtu": {},
		"jc": {}, "jmin": {}, "jmax": {},
		"s1": {}, "s2": {}, "s3": {}, "s4": {},
		"h1": {}, "h2": {}, "h3": {}, "h4": {},
		"i1": {}, "i2": {}, "i3": {}, "i4": {}, "i5": {},
		"headerprotectionkey": {}, "contentpaddingaddition": {},
		"rekeyaftertime": {}, "rekeytimeout": {}, "rejectaftertime": {},
		"keepalivetimeout": {}, "maxhandshakeattempts": {},
		"randomtrailers": {}, "disablecookies": {},
	},
	"peer": {
		"publickey": {}, "presharedkey": {}, "endpoint": {},
		"allowedips": {}, "persistentkeepalive": {},
	},
}

func parseConfig(reader io.Reader) (AWGConfig, error) {
	sections := map[string]map[string]string{}
	sectionCounts := map[string]int{}
	current := ""
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(stripConfigComment(scanner.Text()))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			current = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			if _, ok := knownConfigKeys[current]; !ok {
				return AWGConfig{}, fmt.Errorf("line %d: unsupported section [%s]", lineNumber, current)
			}
			sectionCounts[current]++
			if sectionCounts[current] > 1 {
				return AWGConfig{}, fmt.Errorf("line %d: multiple [%s] sections are not supported", lineNumber, current)
			}
			sections[current] = map[string]string{}
			continue
		}
		if current == "" {
			return AWGConfig{}, fmt.Errorf("line %d: parameter outside a section", lineNumber)
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return AWGConfig{}, fmt.Errorf("line %d: expected key=value", lineNumber)
		}
		originalKey := strings.TrimSpace(key)
		key = strings.ToLower(originalKey)
		if _, ok := knownConfigKeys[current][key]; !ok {
			return AWGConfig{}, fmt.Errorf("line %d: unknown parameter %q in [%s]", lineNumber, originalKey, current)
		}
		if _, exists := sections[current][key]; exists {
			return AWGConfig{}, fmt.Errorf("line %d: duplicate parameter %q in [%s]", lineNumber, originalKey, current)
		}
		sections[current][key] = strings.TrimSpace(value)
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
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			return AWGConfig{}, fmt.Errorf("invalid Interface MTU %q", value)
		}
		mtu = parsed
	}

	//	cfg := AWGConfig{
	//		PrivateKey: iface["privatekey"],
	//		PublicKey:  peer["publickey"],
	//		Endpoint:   peer["endpoint"],
	//		Addresses:  splitCSV(iface["address"]),
	//		DNS:        splitCSV(iface["dns"]),
	//		MTU:        mtu,
	//		AllowedIPs: splitCSV(peer["allowedips"]),
	//		Params:     map[string]string{},
	//	}

	cfg := AWGConfig{
		PrivateKey:          iface["privatekey"],
		ListenPort:          iface["listenport"],
		PublicKey:           peer["publickey"],
		PresharedKey:        peer["presharedkey"],
		Endpoint:            peer["endpoint"],
		PersistentKeepalive: peer["persistentkeepalive"],
		Addresses:           splitCSV(iface["address"]),
		DNS:                 splitCSV(iface["dns"]),
		MTU:                 mtu,
		AllowedIPs:          splitCSV(peer["allowedips"]),
		Params:              map[string]string{},
	}
	if len(cfg.DNS) == 0 {
		cfg.DNS = []string{"1.1.1.1"}
	}

	for _, key := range []string{
		"jc", "jmin", "jmax",
		"s1", "s2", "s3", "s4",
		"h1", "h2", "h3", "h4",
		"i1", "i2", "i3", "i4", "i5",
		"headerprotectionkey",
		"contentpaddingaddition",
		"rekeyaftertime",
		"rekeytimeout",
		"rejectaftertime",
		"keepalivetimeout",
		"maxhandshakeattempts",
		"randomtrailers",
		"disablecookies",
	} {
		if value := iface[key]; value != "" {
			cfg.Params[key] = value
		}
	}

	if err := validateConfig(cfg); err != nil {
		return AWGConfig{}, err
	}

	return cfg, nil
}

func stripConfigComment(line string) string {
	depth := 0
	var previous rune
	for index, current := range line {
		switch current {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		case '#', ';':
			if depth == 0 && (index == 0 || unicode.IsSpace(previous)) {
				return line[:index]
			}
		}
		previous = current
	}
	return line
}

func validateConfig(cfg AWGConfig) error {
	if cfg.PrivateKey == "" {
		return fmt.Errorf("missing Interface PrivateKey")
	}
	if _, err := base64KeyToHex(cfg.PrivateKey); err != nil {
		return fmt.Errorf("invalid Interface PrivateKey: %w", err)
	}
	if cfg.PublicKey == "" {
		return fmt.Errorf("missing Peer PublicKey")
	}
	if _, err := base64KeyToHex(cfg.PublicKey); err != nil {
		return fmt.Errorf("invalid Peer PublicKey: %w", err)
	}
	if cfg.PresharedKey != "" {
		if _, err := base64KeyToHex(cfg.PresharedKey); err != nil {
			return fmt.Errorf("invalid Peer PresharedKey: %w", err)
		}
	}
	if cfg.Endpoint == "" {
		return fmt.Errorf("missing Peer Endpoint")
	}
	if err := validateEndpointSyntax(cfg.Endpoint); err != nil {
		return fmt.Errorf("invalid Peer Endpoint: %w", err)
	}
	if len(cfg.Addresses) == 0 {
		return fmt.Errorf("missing Interface Address")
	}
	if _, err := parseAddrList(cfg.Addresses); err != nil {
		return fmt.Errorf("invalid Interface Address: %w", err)
	}
	if len(cfg.AllowedIPs) == 0 {
		return fmt.Errorf("missing Peer AllowedIPs")
	}
	for _, allowedIP := range cfg.AllowedIPs {
		if _, err := netip.ParsePrefix(allowedIP); err != nil {
			return fmt.Errorf("invalid Peer AllowedIPs entry %q: %w", allowedIP, err)
		}
	}
	if cfg.MTU <= 0 {
		return fmt.Errorf("invalid Interface MTU %d", cfg.MTU)
	}
	if cfg.ListenPort != "" {
		if _, err := strconv.ParseUint(cfg.ListenPort, 10, 16); err != nil {
			return fmt.Errorf("invalid Interface ListenPort: %w", err)
		}
	}
	if cfg.PersistentKeepalive != "" {
		if err := validateUintRange(cfg.PersistentKeepalive); err != nil {
			return fmt.Errorf("invalid Peer PersistentKeepalive: %w", err)
		}
	}

	for _, key := range []string{"jc", "jmin", "jmax"} {
		if value := cfg.Params[key]; value != "" {
			if _, err := strconv.ParseUint(value, 10, 32); err != nil {
				return fmt.Errorf("invalid %s: %w", key, err)
			}
		}
	}
	paddingValues := [4]uint64{}
	for i, key := range []string{"s1", "s2", "s3", "s4"} {
		if value := cfg.Params[key]; value != "" {
			padding, err := strconv.ParseUint(value, 10, 16)
			if err != nil {
				return fmt.Errorf("invalid %s: %w", key, err)
			}
			paddingValues[i] = padding
		}
	}
	for _, key := range []string{
		"h1", "h2", "h3", "h4",
		"contentpaddingaddition",
		"rekeyaftertime",
		"rekeytimeout",
		"rejectaftertime",
		"keepalivetimeout",
		"maxhandshakeattempts",
	} {
		if value := cfg.Params[key]; value != "" {
			if err := validateUintRange(value); err != nil {
				return fmt.Errorf("invalid %s: %w", key, err)
			}
		}
	}
	if value := cfg.Params["headerprotectionkey"]; value != "" {
		headerKey, err := base64KeyToHex(value)
		if err != nil {
			return fmt.Errorf("invalid HeaderProtectionKey: %w", err)
		}
		if strings.Trim(headerKey, "0") != "" {
			for i, padding := range paddingValues {
				if padding < device.HeaderCipherNonceSize {
					return fmt.Errorf(
						"invalid s%d: must be at least %d when HeaderProtectionKey is set",
						i+1,
						device.HeaderCipherNonceSize,
					)
				}
			}
		}
	}
	for _, key := range []string{"randomtrailers", "disablecookies"} {
		if value := cfg.Params[key]; value != "" {
			if _, err := awgBoolToUAPI(value); err != nil {
				return fmt.Errorf("invalid %s: %w", key, err)
			}
		}
	}

	jmin, hasJmin := cfg.Params["jmin"]
	jmax, hasJmax := cfg.Params["jmax"]
	if hasJmin && hasJmax {
		min, _ := strconv.ParseUint(jmin, 10, 32)
		max, _ := strconv.ParseUint(jmax, 10, 32)
		if min > max {
			return fmt.Errorf("invalid junk size range: Jmin must not exceed Jmax")
		}
	}

	var headerRanges [4]device.UintRange
	for i := range headerRanges {
		headerRanges[i].FromUint32(uint32(i+1), uint32(i+1))
		if value := cfg.Params[fmt.Sprintf("h%d", i+1)]; value != "" {
			_ = headerRanges[i].FromString(value)
		}
	}
	for i := 0; i < len(headerRanges); i++ {
		for j := i + 1; j < len(headerRanges); j++ {
			if headerRanges[i].Overlap(headerRanges[j]) {
				return fmt.Errorf("invalid header ranges: H%d and H%d overlap", i+1, j+1)
			}
		}
	}

	return nil
}

func validateUintRange(value string) error {
	var valueRange device.UintRange
	return valueRange.FromString(value)
}

func validateEndpointSyntax(value string) error {
	host, port, err := net.SplitHostPort(value)
	if err != nil {
		return err
	}
	if host == "" {
		return fmt.Errorf("host is empty")
	}
	if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return fmt.Errorf("invalid port: %w", err)
	}
	return nil
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
