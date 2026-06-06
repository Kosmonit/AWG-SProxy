package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/awg-sproxy/awg-sproxy/proxy"
)

const appName = "AWG-SProxy"

func main() {
	configPath := flag.String("config", "config.conf", "AmneziaWG config file path")
	endpointOverride := flag.String("endpoint", "", "override Peer Endpoint from config (e.g. 8.6.112.208:7281)")
	socksPort := flag.Int("socks", 8600, "SOCKS5 proxy listen port (0 to disable)")
	httpPort := flag.Int("http", 8601, "HTTP proxy listen port (0 to disable)")
	bindAddr := flag.String("bind", "127.0.0.1", "Address to bind proxy listeners")
	flag.Parse()

	logger := log.New(os.Stdout, "", log.LstdFlags)

	cfg, err := loadConfig(*configPath)
	if err != nil {
		logger.Fatalf("%s: %v", appName, err)
	}
	if *endpointOverride != "" {
		cfg.Endpoint = *endpointOverride
	}

	tunnel, err := startTunnel(cfg)
	if err != nil {
		logger.Fatalf("%s: tunnel failed: %v", appName, err)
	}
	defer tunnel.Close()

	dialer := &proxy.TunnelDialer{Net: tunnel.Net}

	socksAddr := fmt.Sprintf("%s:%d", *bindAddr, *socksPort)
	httpAddr := fmt.Sprintf("%s:%d", *bindAddr, *httpPort)

	logger.Printf("%s started", appName)
	logger.Printf("  endpoint: %s", cfg.Endpoint)
	logger.Printf("  dns:      %s", strings.Join(cfg.DNS, ", "))
	logger.Printf("  tunnel:   userspace netstack (no system interface)")

	errCh := make(chan error, 2)

	if *socksPort > 0 {
		srv := &proxy.SOCKS5Server{
			Addr:   socksAddr,
			Dialer: dialer,
			Log:    logger,
		}
		go func() {
			errCh <- srv.ListenAndServe()
		}()
	} else {
		logger.Printf("  socks5:   disabled")
	}

	if *httpPort > 0 {
		srv := &proxy.HTTPServer{
			Addr:   httpAddr,
			Dialer: dialer,
			Log:    logger,
		}
		go func() {
			errCh <- srv.ListenAndServe()
		}()
	} else {
		logger.Printf("  http:     disabled")
	}

	if *socksPort > 0 {
		logger.Printf("  socks5:   %s", socksAddr)
	}
	if *httpPort > 0 {
		logger.Printf("  http:     %s", httpAddr)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		logger.Printf("%s: received %v, shutting down", appName, sig)
	case err := <-errCh:
		logger.Fatalf("%s: proxy error: %v", appName, err)
	}
}
