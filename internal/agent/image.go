package agent

import (
	"fmt"
	"strings"
)

// ImageAllowed reports whether spec.agent.image may be used.
// The empty image (DefaultImage) is always allowed; extra is an exact-match allowlist.
func ImageAllowed(image string, extra []string) bool {
	if image == "" || image == DefaultImage {
		return true
	}
	for _, e := range extra {
		if strings.TrimSpace(e) == image {
			return true
		}
	}
	return false
}

// ValidateImage returns an error if env.Spec.Agent.Image is not allowlisted.
func ValidateImage(image string, extra []string) error {
	if ImageAllowed(image, extra) {
		return nil
	}
	return fmt.Errorf("agent image %q is not allowlisted", image)
}
