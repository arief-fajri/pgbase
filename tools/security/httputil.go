package security

import (
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

// SafeHTTPClient initializes a custom http.Client with extra host checks to
// prevent internal network probing requests (aka. disallow loopback, private,
// multicast, etc. requests).
//
// NB! The host checks are not perfect and there are probably edge cases that are
// not covered, so if you plan using it with untrusted user URL, consider
// performing additional whitelist checks.
func SafeHTTPClient() *http.Client {
	dialer := &net.Dialer{
		// the same options as in http.DefaultTransport.DialContext
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,

		// check the address right after establishing the connection to prevent dns rebinding
		Control: func(network, address string, c syscall.RawConn) error {
			return ValidateDialAddress(address)
		},
	}

	return &http.Client{
		Timeout: 180 * time.Second, // can be still cancelled with the request context
		Transport: &http.Transport{
			DialContext: dialer.DialContext,
			// the same options as in http.DefaultTransport
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}

// isDisallowedIP reports whether the given IP should never be dialed
// (loopback, unspecified, private, link-local unicast/multicast, or multicast).
func isDisallowedIP(ip net.IP) bool {
	return ip == nil ||
		ip.IsLoopback() ||
		ip.IsUnspecified() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast()
}

// ValidateDialAddress rejects dialing to loopback, unspecified, private,
// link-local unicast/multicast, and multicast addresses.
func ValidateDialAddress(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}

	ip := net.ParseIP(host)
	if isDisallowedIP(ip) {
		return fmt.Errorf("address %q is invalid or resolve to disallowed IP", address)
	}

	return nil
}