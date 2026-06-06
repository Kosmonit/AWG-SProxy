package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

type HTTPServer struct {
	Addr   string
	Dialer *TunnelDialer
	Log    *log.Logger
}

func (s *HTTPServer) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}
	s.Log.Printf("HTTP proxy listening on %s", s.Addr)
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go s.handle(conn)
	}
}

func (s *HTTPServer) handle(conn net.Conn) {
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	reader := bufio.NewReader(conn)
	req, err := http.ReadRequest(reader)
	if err != nil {
		s.Log.Printf("http proxy read: %v", err)
		return
	}
	_ = conn.SetDeadline(time.Time{})

	if req.Method == http.MethodConnect {
		s.handleConnect(conn, req)
		return
	}

	s.handleHTTP(conn, req)
}

func (s *HTTPServer) handleConnect(conn net.Conn, req *http.Request) {
	target := req.Host
	if !strings.Contains(target, ":") {
		target = net.JoinHostPort(target, "443")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	remote, err := s.Dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		s.Log.Printf("http connect %s: %v", target, err)
		_, _ = fmt.Fprintf(conn, "HTTP/1.1 502 Bad Gateway\r\n\r\n")
		return
	}
	defer remote.Close()

	_, _ = fmt.Fprintf(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
	Relay(conn, remote)
}

func (s *HTTPServer) handleHTTP(conn net.Conn, req *http.Request) {
	if req.URL == nil || !req.URL.IsAbs() {
		_, _ = fmt.Fprintf(conn, "HTTP/1.1 400 Bad Request\r\n\r\n")
		return
	}

	target := req.URL.Host
	if !strings.Contains(target, ":") {
		if req.URL.Scheme == "https" {
			target = net.JoinHostPort(target, "443")
		} else {
			target = net.JoinHostPort(target, "80")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	remote, err := s.Dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		s.Log.Printf("http proxy %s: %v", target, err)
		_, _ = fmt.Fprintf(conn, "HTTP/1.1 502 Bad Gateway\r\n\r\n")
		return
	}
	defer remote.Close()

	req.RequestURI = ""
	if req.Header.Get("Proxy-Connection") != "" {
		req.Header.Del("Proxy-Connection")
	}
	if err := req.Write(remote); err != nil {
		return
	}

	reader := bufio.NewReader(remote)
	resp, err := http.ReadResponse(reader, req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	_ = resp.Write(conn)
	_, _ = io.Copy(conn, resp.Body)
}
