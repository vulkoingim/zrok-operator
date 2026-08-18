package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/openziti/zrok/v2/agent/agentGrpc"

	zrokv1alpha1 "github.com/vulkoingim/zrok-operator/api/v1alpha1"
	"github.com/vulkoingim/zrok-operator/internal/zrokclient"
)

const (
	annotationAppliedDigest = "zrok.k8s.zrok.io/applied-digest"
	annotationReclaimName   = "zrok.k8s.zrok.io/reclaim-name"
)

func desiredShareMode(share *zrokv1alpha1.ZrokShare) string {
	if share.Spec.ShareMode == "" {
		return string(zrokv1alpha1.ShareModePublic)
	}
	return string(share.Spec.ShareMode)
}

func desiredBackendMode(share *zrokv1alpha1.ZrokShare) string {
	if share.Spec.BackendMode == "" {
		return string(zrokv1alpha1.BackendModeProxy)
	}
	return string(share.Spec.BackendMode)
}

// agentShareMatchesSpec reports whether the live agent share matches identity fields
// we can observe: target, frontend name, shareMode, backendMode, closed, private token.
func agentShareMatchesSpec(share *zrokv1alpha1.ZrokShare, d *agentGrpc.ShareDetail, desiredTarget string) bool {
	if d == nil || !isAgentShareActive(d) || !agentTargetOK(d, desiredTarget) {
		return false
	}
	if mode := d.GetShareMode(); mode != "" && !strings.EqualFold(mode, desiredShareMode(share)) {
		return false
	}
	if bm := d.GetBackendMode(); bm != "" && !strings.EqualFold(bm, desiredBackendMode(share)) {
		return false
	}
	if d.GetClosed() != share.Spec.Closed {
		return false
	}
	if want := share.Spec.PrivateShareToken; want != "" && d.GetToken() != want {
		return false
	}
	name := reservedFrontendName(share)
	if name == "" {
		// Dropping nameSelection must rebuild; otherwise we'd keep serving the reserved URL.
		return share.Status.Reservation != zrokv1alpha1.ReservationReserved
	}
	return agentFrontendMatchesName(d, name)
}

func agentFrontendMatchesName(d *agentGrpc.ShareDetail, name string) bool {
	if d == nil || name == "" {
		return false
	}
	for _, ep := range d.GetFrontendEndpoint() {
		if zrokclient.FrontendEndpointMatchesName(ep, name) {
			return true
		}
	}
	return false
}

func liveFrontendName(share *zrokv1alpha1.ZrokShare, cls classifiedShare) string {
	if d := cls.agent; d != nil {
		if n := zrokclient.FrontendNameFromEndpoints(d.GetFrontendEndpoint()); n != "" {
			return n
		}
	}
	if rem := cls.remote; rem != nil {
		if n := zrokclient.FrontendNameFromEndpoints(rem.FrontendEndpoints); n != "" {
			return n
		}
	}
	return zrokclient.FrontendNameFromEndpoint(share.Status.AssignedURL)
}

func shareApplyDigest(share *zrokv1alpha1.ZrokShare, basicAuth []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "env=%s\n", share.Spec.EnvironmentRef.Name)
	fmt.Fprintf(&b, "mode=%s\n", desiredShareMode(share))
	fmt.Fprintf(&b, "backend=%s\n", desiredBackendMode(share))
	fmt.Fprintf(&b, "up=%s\n", share.Spec.Upstream.URL)
	fmt.Fprintf(&b, "name=%s/%s\n", nameNamespaceToken(share), reservedFrontendName(share))
	fmt.Fprintf(&b, "priv=%s\n", share.Spec.PrivateShareToken)
	fmt.Fprintf(&b, "insecure=%t\nclosed=%t\n", share.Spec.Insecure, share.Spec.Closed)
	grants := append([]string(nil), share.Spec.AccessGrants...)
	sort.Strings(grants)
	for _, g := range grants {
		fmt.Fprintf(&b, "grant=%s\n", g)
	}
	if ref := share.Spec.BasicAuthSecretRef; ref != nil {
		fmt.Fprintf(&b, "ba-secret=%s\n", ref.Name)
	}
	for _, ba := range basicAuth {
		fmt.Fprintf(&b, "ba=%s\n", ba)
	}
	if o := share.Spec.OAuth; o != nil {
		fmt.Fprintf(&b, "oauth=%s\ninterval=%s\n", o.Provider, o.RefreshInterval)
		domains := append([]string(nil), o.EmailDomains...)
		sort.Strings(domains)
		for _, d := range domains {
			fmt.Fprintf(&b, "oauth-domain=%s\n", d)
		}
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:16])
}

func shareDigestDrifted(share *zrokv1alpha1.ZrokShare, digest string) bool {
	if share.Annotations == nil {
		return false
	}
	applied := share.Annotations[annotationAppliedDigest]
	if applied == "" {
		return false
	}
	return applied != digest
}

func adoptableAgentShare(
	share *zrokv1alpha1.ZrokShare,
	cls classifiedShare,
	desiredTarget, digest string,
) *agentGrpc.ShareDetail {
	d := cls.agent
	if d == nil || !agentShareMatchesSpec(share, d, desiredTarget) || shareDigestDrifted(share, digest) {
		return nil
	}
	return d
}
