package zrokclient

import (
	"net/url"
	"strings"
)

// RemoteShare is a controller-API share (Metadata.ListShares), including Target.
// zrok shares have no description/labels — identity is name + target + envZID.
type RemoteShare struct {
	Token             string
	Target            string
	ShareMode         string
	BackendMode       string
	FrontendEndpoints []string
}

// MatchesFrontendName reports whether any frontend URL belongs to the reserved share name.
func (s RemoteShare) MatchesFrontendName(name string) bool {
	for _, ep := range s.FrontendEndpoints {
		if FrontendEndpointMatchesName(ep, name) {
			return true
		}
	}
	return false
}

// FindByToken returns the share with the given token, or nil.
func FindByToken(shares []RemoteShare, token string) *RemoteShare {
	if token == "" {
		return nil
	}
	for i := range shares {
		if shares[i].Token == token {
			return &shares[i]
		}
	}
	return nil
}

// FindByFrontendName returns the share whose frontend URL matches the reserved name, or nil.
func FindByFrontendName(shares []RemoteShare, name string) *RemoteShare {
	if name == "" {
		return nil
	}
	for i := range shares {
		if shares[i].MatchesFrontendName(name) {
			return &shares[i]
		}
	}
	return nil
}

// FrontendEndpointMatchesName reports whether a frontend URL belongs to the reserved share name.
func FrontendEndpointMatchesName(endpoint, name string) bool {
	if endpoint == "" || name == "" {
		return false
	}
	host := frontendHost(endpoint)
	return strings.HasPrefix(host, strings.ToLower(name)+".")
}

// FrontendNameFromEndpoint returns the reserved-name DNS label (the part before
// `.shares.zrok.io`). Empty if endpoint is empty or has no host.
func FrontendNameFromEndpoint(endpoint string) string {
	host := frontendHost(endpoint)
	if host == "" {
		return ""
	}
	if i := strings.IndexByte(host, '.'); i > 0 {
		return host[:i]
	}
	return host
}

// FrontendNameFromEndpoints returns the first parseable frontend name.
func FrontendNameFromEndpoints(endpoints []string) string {
	for _, ep := range endpoints {
		if n := FrontendNameFromEndpoint(ep); n != "" {
			return n
		}
	}
	return ""
}

func frontendHost(endpoint string) string {
	host := strings.ToLower(strings.TrimSpace(endpoint))
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	if i := strings.IndexByte(host, '/'); i >= 0 {
		host = host[:i]
	}
	if i := strings.IndexByte(host, ':'); i >= 0 {
		// port on a hostname (not IPv6)
		if !strings.Contains(host, "]") {
			host = host[:i]
		}
	}
	return host
}

// TargetsEqual reports whether two upstream/target URLs refer to the same backend.
func TargetsEqual(a, b string) bool {
	return normalizeTarget(a) == normalizeTarget(b)
}

func normalizeTarget(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return strings.TrimRight(strings.ToLower(s), "/")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Host)
	switch {
	case u.Scheme == schemeHTTP && strings.HasSuffix(host, ":80"):
		host = strings.TrimSuffix(host, ":80")
	case u.Scheme == schemeHTTPS && strings.HasSuffix(host, ":443"):
		host = strings.TrimSuffix(host, ":443")
	}
	u.Host = host
	u.Path = strings.TrimRight(u.Path, "/")
	u.Fragment = ""
	return u.String()
}
