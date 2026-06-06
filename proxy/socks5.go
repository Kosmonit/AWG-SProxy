package proxy

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"time"
)

const (
	socksVersion5       = 0x05
	socksCmdConnect     = 0x01
	socksAddrIPv4       = 0x01
	socksAddrDomainName = 0x03
	socksAddrIPv6       = 0x04
	socksRepSuccess     = 0x00
	socksRepGeneralFail = 0x01
	socksRepHostUnreach = 0x04
	socksRepCmdNotSup   = 0x07
	socksRepAddrNotSup  = 0x08
)

type SOCKS5Server struct {
	Addr   string
	Dialer *TunnelDialer
	Log    *log.Logger
}

func (s *SOCKS5Server) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}
	s.Log.Printf("SOCKS5 listening on %s", s.Addr)
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go s.handle(conn)
	}
}

func (s *SOCKS5Server) handle(conn net.Conn) {
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	if err := s.negotiate(conn); err != nil {
		s.Log.Printf("socks5 handshake: %v", err)
		return
	}
	_ = conn.SetDeadline(time.Time{})

	target, err := s.readRequest(conn)
	if err != nil {
		s.Log.Printf("socks5 request: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	remote, err := s.Dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		s.Log.Printf("socks5 dial %s: %v", target, err)
		_ = writeSocksReply(conn, socksRepHostUnreach)
		return
	}
	defer remote.Close()

	if err := writeSocksReply(conn, socksRepSuccess); err != nil {
		return
	}

	Relay(conn, remote)
}

func (s *SOCKS5Server) negotiate(conn net.Conn) error {
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return err
	}
	if header[0] != socksVersion5 {
		return errors.New("unsupported socks version")
	}
	nMethods := int(header[1])
	methods := make([]byte, nMethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return err
	}

	// No authentication.
	_, err := conn.Write([]byte{socksVersion5, 0x00})
	return err
}

func (s *SOCKS5Server) readRequest(conn net.Conn) (string, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", err
	}
	if header[0] != socksVersion5 {
		return "", errors.New("unsupported socks version")
	}
	if header[1] != socksCmdConnect {
		_ = writeSocksReply(conn, socksRepCmdNotSup)
		return "", fmt.Errorf("unsupported command %d", header[1])
	}

	var host string
	switch header[3] {
	case socksAddrIPv4:
		addr := make([]byte, 4)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", err
		}
		host = net.IP(addr).String()
	case socksAddrDomainName:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return "", err
		}
		domain := make([]byte, lenBuf[0])
		if _, err := io.ReadFull(conn, domain); err != nil {
			return "", err
		}
		host = string(domain)
	case socksAddrIPv6:
		addr := make([]byte, 16)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", err
		}
		host = net.IP(addr).String()
	default:
		_ = writeSocksReply(conn, socksRepAddrNotSup)
		return "", fmt.Errorf("unsupported address type %d", header[3])
	}

	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return "", err
	}
	port := binary.BigEndian.Uint16(portBuf)

	return net.JoinHostPort(host, fmt.Sprintf("%d", port)), nil
}

func writeSocksReply(conn net.Conn, rep byte) error {
	// VER REP RSV ATYP BND.ADDR BND.PORT — bind address unused (zeros).
	reply := []byte{socksVersion5, rep, 0x00, socksAddrIPv4, 0, 0, 0, 0, 0, 0}
	_, err := conn.Write(reply)
	return err
}
