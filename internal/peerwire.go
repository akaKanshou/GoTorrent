package internal

import (
	"errors"
	"net"
)

// initListener inits a *net.TCPListener, trying the range of ports 6881-6889
// Returns the listener and the port it is listening on.
func initListener() (*net.TCPListener, uint16, error) {
	for port := 6881; port <= 6889; port++ {
		listener, err := net.ListenTCP("tcp4", &net.TCPAddr{Port: port})
		if err == nil {
			return listener, uint16(port), err
		}
	}
	return nil, 0, listenerInitFailed
}

var (
	listenerInitFailed = errors.New("failed to create tcp listener")
)
