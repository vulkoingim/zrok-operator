package zrokclient

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxRedirects = 5

// EndpointNotAllowedError is returned when spec.apiEndpoint fails validation.
type EndpointNotAllowedError struct {
	Endpoint string
	Reason   string
}

func (e *EndpointNotAllowedError) Error() string {
	return fmt.Sprintf("apiEndpoint %q: %s", e.Endpoint, e.Reason)
}

// IsEndpointNotAllowed reports whether err is or wraps EndpointNotAllowedError.
func IsEndpointNotAllowed(err error) bool {
	var e *EndpointNotAllowedError
	return errors.As(err, &e)
}

// DefaultAPIHost is the hostname of DefaultAPIEndpoint (always allowlisted).
func DefaultAPIHost() string {
	u, err := url.Parse(DefaultAPIEndpoint)
	if err != nil || u.Hostname() == "" {
		return "api-v2.zrok.io"
	}
	return strings.ToLower(u.Hostname())
}

// NormalizeAPIHosts lowercases extra hosts and always includes DefaultAPIHost.
func NormalizeAPIHosts(extra []string) []string {
	seen := map[string]struct{}{DefaultAPIHost(): {}}
	out := []string{DefaultAPIHost()}
	for _, h := range extra {
		h = strings.ToLower(strings.TrimSpace(h))
		h = strings.TrimPrefix(h, "https://")
		h = strings.TrimPrefix(h, "http://")
		h = strings.TrimRight(h, "/")
		if h == "" {
			continue
		}
		if _, ok := seen[h]; ok {
			continue
		}
		seen[h] = struct{}{}
		out = append(out, h)
	}
	return out
}

// ValidateAPIEndpoint requires https, no userinfo, an allowlisted host, and
// rejects loopback / link-local / cloud-metadata destinations.
func ValidateAPIEndpoint(apiEndpoint string, allowedHosts []string) error {
	endpoint := strings.TrimRight(strings.TrimSpace(apiEndpoint), "/")
	if endpoint == "" {
		endpoint = DefaultAPIEndpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return &EndpointNotAllowedError{Endpoint: apiEndpoint, Reason: "parse failed: " + err.Error()}
	}
	if u.Scheme != schemeHTTPS {
		return &EndpointNotAllowedError{Endpoint: endpoint, Reason: "must use https"}
	}
	if u.User != nil {
		return &EndpointNotAllowedError{Endpoint: endpoint, Reason: "must not contain userinfo"}
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return &EndpointNotAllowedError{Endpoint: endpoint, Reason: "missing host"}
	}
	if blockedHost(host) {
		return &EndpointNotAllowedError{Endpoint: endpoint, Reason: "host is blocked"}
	}
	key := host
	if p := u.Port(); p != "" && p != "443" {
		key = host + ":" + p
	}
	allowed := NormalizeAPIHosts(allowedHosts)
	for _, a := range allowed {
		if a == key || a == host {
			return nil
		}
	}
	return &EndpointNotAllowedError{Endpoint: endpoint, Reason: "host is not allowlisted"}
}

func blockedHost(host string) bool {
	switch host {
	case "localhost", "metadata.google.internal", "metadata.google.internal.":
		return true
	}
	if strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

// NewSecureHTTPClient is the REST transport: timeouts, no InsecureSkipVerify,
// drops X-TOKEN on any redirect, refuses cross-host and non-https redirects.
func NewSecureHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 60 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			req.Header.Del("X-TOKEN")
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			if req.URL.Scheme != schemeHTTPS {
				return fmt.Errorf("refusing non-https redirect to %s", req.URL.Redacted())
			}
			orig := via[0].URL
			if !hostsEqual(orig, req.URL) {
				return fmt.Errorf("refusing cross-host redirect from %s to %s", orig.Host, req.URL.Host)
			}
			return nil
		},
	}
}

func hostsEqual(a, b *url.URL) bool {
	return strings.EqualFold(a.Hostname(), b.Hostname()) && effectivePort(a) == effectivePort(b)
}

func effectivePort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	switch u.Scheme {
	case schemeHTTPS:
		return "443"
	case schemeHTTP:
		return "80"
	default:
		return ""
	}
}
