package httpapi

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// IPSet is a list of addresses and networks.
type IPSet struct {
	prefixes []netip.Prefix
}

// ParseIPSet reads a comma or space separated list of addresses and CIDR networks.
// An unusable entry is an error: dropping it could silently widen or narrow an
// access rule.
func ParseIPSet(raw string) (*IPSet, error) {
	set := &IPSet{}
	for _, entry := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
		if strings.Contains(entry, "/") {
			prefix, err := netip.ParsePrefix(entry)
			if err != nil {
				return nil, fmt.Errorf("invalid network %q: %w", entry, err)
			}
			set.prefixes = append(set.prefixes, prefix.Masked())
			continue
		}
		addr, err := netip.ParseAddr(entry)
		if err != nil {
			return nil, fmt.Errorf("invalid address %q: %w", entry, err)
		}
		addr = addr.Unmap()
		set.prefixes = append(set.prefixes, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return set, nil
}

// Empty reports whether the set has no entry.
func (s *IPSet) Empty() bool { return s == nil || len(s.prefixes) == 0 }

// Contains reports whether the address belongs to the set.
func (s *IPSet) Contains(addr netip.Addr) bool {
	if s == nil || !addr.IsValid() {
		return false
	}
	addr = addr.Unmap()
	for _, prefix := range s.prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// peerAddr parses the address of the TCP peer.
func peerAddr(remote string) netip.Addr {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	if i := strings.IndexByte(host, '%'); i >= 0 {
		host = host[:i]
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return addr.Unmap()
}

// clientIP returns the caller address. X-Forwarded-For is only honoured when the
// TCP peer is a configured trusted proxy, and it is read from the right: the first
// address that is not itself a trusted proxy is the client, whatever the client
// prepended on its own.
func (s *Server) clientIP(r *http.Request) string {
	peer := peerAddr(r.RemoteAddr)
	if !s.trustedProxies.Contains(peer) {
		if !peer.IsValid() {
			return ""
		}
		return peer.String()
	}
	hops := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		addr, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break
		}
		addr = addr.Unmap()
		if !s.trustedProxies.Contains(addr) {
			return addr.String()
		}
	}
	return peer.String()
}

// AllowListener closes connections from addresses outside the set before any byte
// is read, so a refused client cannot reach TLS, HTTP or the application at all.
type AllowListener struct {
	net.Listener
	Allowed *IPSet
	// OnRefused is told about each refused address (optional).
	OnRefused func(addr string)
}

// Accept implements net.Listener.
func (l *AllowListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if l.Allowed.Empty() || l.Allowed.Contains(peerAddr(conn.RemoteAddr().String())) {
			return conn, nil
		}
		if l.OnRefused != nil {
			l.OnRefused(conn.RemoteAddr().String())
		}
		_ = conn.Close()
	}
}
