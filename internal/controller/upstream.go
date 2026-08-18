package controller

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

var dnsLabelRE = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// validateShareUpstream requires the URL host to be a Service in namespace
// (short name, {svc}.{ns}, {svc}.{ns}.svc, or {svc}.{ns}.svc.cluster.local).
func validateShareUpstream(rawURL, namespace string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse upstream: %w", err)
	}
	switch u.Scheme {
	case "http", "https", "tcp", "udp":
	default:
		return fmt.Errorf("unsupported upstream scheme %q", u.Scheme)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return fmt.Errorf("upstream missing host")
	}
	if net.ParseIP(host) != nil {
		return fmt.Errorf("upstream IP addresses are not allowed")
	}
	ns := strings.ToLower(namespace)
	suffixes := []string{
		"." + ns + ".svc.cluster.local",
		"." + ns + ".svc",
		"." + ns,
	}
	for _, suf := range suffixes {
		if before, ok := strings.CutSuffix(host, suf); ok {
			name := before
			if isDNSLabel(name) {
				return nil
			}
		}
	}
	if !strings.Contains(host, ".") && isDNSLabel(host) {
		return nil
	}
	return fmt.Errorf("upstream host %q is not a Service in namespace %q", host, namespace)
}

func isDNSLabel(s string) bool {
	return len(s) >= 1 && len(s) <= 63 && dnsLabelRE.MatchString(s)
}
