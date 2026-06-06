package proxy

import (
	"io"
	"net"
	"sync"
)

func Relay(client, remote net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	copyBoth := func(dst, src net.Conn) {
		defer wg.Done()
		_, _ = io.Copy(dst, src)
		if c, ok := dst.(interface{ CloseWrite() error }); ok {
			_ = c.CloseWrite()
		}
	}
	go copyBoth(remote, client)
	go copyBoth(client, remote)
	wg.Wait()
}
