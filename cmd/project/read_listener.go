package main

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
)

// The optional peer listener preserves Open's principal, fixed revision and
// require_write checks without granting access to mutation or MCP routes.
func validateReadListener(business, protocol, read string, secure bool, token string, otherTokens []string) error {
	if read == "" && token == "" {
		return nil
	}
	invalid := errors.New("separate authenticated read listener required")
	if read == "" || len(token) < 32 || strings.ContainsAny(token, " \r\n\t") {
		return invalid
	}
	for _, other := range otherTokens {
		if token == other {
			return invalid
		}
	}
	host, port, err := net.SplitHostPort(read)
	n, parseErr := strconv.Atoi(port)
	if err != nil || parseErr != nil || n < 1 || n > 65535 {
		return invalid
	}
	if !secure {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return invalid
		}
	}
	for _, addr := range []string{business, protocol} {
		if addr == "" {
			continue
		}
		_, otherPort, splitErr := net.SplitHostPort(addr)
		other, numberErr := strconv.Atoi(otherPort)
		if splitErr != nil || numberErr != nil || other == n {
			return invalid
		}
	}
	return nil
}

func projectReadHandler(open, health http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawPath == "" && r.URL.RawQuery == "" && !r.URL.ForceQuery {
			switch {
			case r.Method == http.MethodPost && r.URL.Path == "/internal/v1/project/open":
				open.ServeHTTP(w, r)
				return
			case r.Method == http.MethodGet && r.URL.Path == "/health":
				health.ServeHTTP(w, r)
				return
			}
		}
		http.NotFound(w, r)
	})
}
