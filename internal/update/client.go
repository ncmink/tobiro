package update

import (
	"net"
	"net/http"
	"time"
)

const (
	dialTimeout           = 15 * time.Second
	tlsHandshakeTimeout   = 15 * time.Second
	responseHeaderTimeout = 30 * time.Second
)

// NewHTTPClient returns a client suited for GitHub API and release asset checks.
func NewHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: 30 * time.Second,
	}).DialContext
	transport.TLSHandshakeTimeout = tlsHandshakeTimeout
	transport.ResponseHeaderTimeout = responseHeaderTimeout
	return &http.Client{Transport: transport}
}
