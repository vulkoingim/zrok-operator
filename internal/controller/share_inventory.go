package controller

import (
	"github.com/openziti/zrok/v2/agent/agentGrpc"

	zrokv1alpha1 "github.com/vulkoingim/zrok-operator/api/v1alpha1"
	"github.com/vulkoingim/zrok-operator/internal/zrokclient"
)

// shareInventory is one reconcile's view of remote controller shares + local agent Status.
type shareInventory struct {
	remote []zrokclient.RemoteShare
	agent  []*agentGrpc.ShareDetail
}

type classifiedShare struct {
	remote      *zrokclient.RemoteShare
	agent       *agentGrpc.ShareDetail
	foreignName bool
}

func (inv shareInventory) remoteByToken(token string) *zrokclient.RemoteShare {
	return zrokclient.FindByToken(inv.remote, token)
}

func (inv shareInventory) remoteByName(name string) *zrokclient.RemoteShare {
	return zrokclient.FindByFrontendName(inv.remote, name)
}

func (inv shareInventory) agentByToken(token string) *agentGrpc.ShareDetail {
	if token == "" {
		return nil
	}
	for _, s := range inv.agent {
		if s != nil && s.GetToken() == token {
			return s
		}
	}
	return nil
}

func (inv shareInventory) agentByName(name string) *agentGrpc.ShareDetail {
	if name == "" {
		return nil
	}
	for _, s := range inv.agent {
		if s == nil || !isAgentShareActive(s) {
			continue
		}
		for _, ep := range s.GetFrontendEndpoint() {
			if zrokclient.FrontendEndpointMatchesName(ep, name) {
				return s
			}
		}
	}
	return nil
}

func reservedFrontendName(share *zrokv1alpha1.ZrokShare) string {
	if share.Spec.NameSelection == nil {
		return ""
	}
	return share.Spec.NameSelection.Name
}

func nameNamespaceToken(share *zrokv1alpha1.ZrokShare) string {
	if share.Spec.NameSelection == nil {
		return ""
	}
	if share.Spec.NameSelection.Namespace == "" {
		return zrokv1alpha1.DefaultNamespaceToken
	}
	return share.Spec.NameSelection.Namespace
}

// classifyShare decides what the reserved name / status token maps to remotely and in the agent.
// foreignName is true when the reserved name is attached to a different target and this CR
// does not already own that share token — never Unshare that holder.
func classifyShare(share *zrokv1alpha1.ZrokShare, desiredTarget string, inv shareInventory) classifiedShare {
	name := reservedFrontendName(share)
	cls := classifiedShare{}

	if tok := share.Status.ShareToken; tok != "" {
		cls.remote = inv.remoteByToken(tok)
		cls.agent = inv.agentByToken(tok)
	}
	if cls.remote == nil && name != "" {
		cls.remote = inv.remoteByName(name)
	}
	if cls.agent == nil && name != "" {
		cls.agent = inv.agentByName(name)
	}
	if cls.remote == nil && share.Spec.PrivateShareToken != "" {
		cls.remote = inv.remoteByToken(share.Spec.PrivateShareToken)
	}

	if name == "" {
		return cls
	}
	holder := inv.remoteByName(name)
	if holder == nil || zrokclient.TargetsEqual(holder.Target, desiredTarget) {
		return cls
	}
	if share.Status.ShareToken == holder.Token {
		// Spec.upstream changed on our own share — still ours; heal will Unshare+recreate.
		return cls
	}
	cls.foreignName = true
	cls.remote = holder
	return cls
}

func agentTargetOK(s *agentGrpc.ShareDetail, desiredTarget string) bool {
	if s == nil || desiredTarget == "" {
		return false
	}
	be := s.GetBackendEndpoint()
	if be == "" {
		return false
	}
	return zrokclient.TargetsEqual(be, desiredTarget)
}

func isOurShareToken(share *zrokv1alpha1.ZrokShare, token string, inv shareInventory) bool {
	if token == "" {
		return false
	}
	if share.Status.ShareToken == token {
		return true
	}
	if rem := inv.remoteByToken(token); rem != nil {
		return zrokclient.TargetsEqual(rem.Target, share.Spec.Upstream.URL)
	}
	// Live in this environment's agent → ours (the agent is not shared across accounts).
	return inv.agentByToken(token) != nil
}

// otherShareWithFrontendName returns another non-deleting ZrokShare that claims the same reserved name.
func otherShareWithFrontendName(self *zrokv1alpha1.ZrokShare, all []zrokv1alpha1.ZrokShare) *zrokv1alpha1.ZrokShare {
	name := reservedFrontendName(self)
	if name == "" {
		return nil
	}
	for i := range all {
		o := &all[i]
		if o.Name == self.Name && o.Namespace == self.Namespace {
			continue
		}
		if !o.DeletionTimestamp.IsZero() {
			continue
		}
		if reservedFrontendName(o) == name {
			return o
		}
	}
	return nil
}
