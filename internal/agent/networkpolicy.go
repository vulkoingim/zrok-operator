package agent

import (
	"k8s.io/apimachinery/pkg/util/intstr"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	zrokv1alpha1 "github.com/vulkoingim/zrok-operator/api/v1alpha1"
)

const (
	// ManagerAppNameLabel is the default app.kubernetes.io/name on the manager pod.
	ManagerAppNameLabel = "zrok-operator"

	// NamespaceNameLabel is the well-known label Kubernetes applies to every Namespace.
	NamespaceNameLabel = "kubernetes.io/metadata.name"
)

// NetworkPolicyName is the per-environment agent NetworkPolicy.
func NetworkPolicyName(env *zrokv1alpha1.ZrokEnvironment) string {
	return ServiceName(env)
}

// DesiredNetworkPolicy allows Ingress to agent gRPC only from manager pods
// in managerNamespace (matched via kubernetes.io/metadata.name + app.kubernetes.io/name).
func DesiredNetworkPolicy(env *zrokv1alpha1.ZrokEnvironment, managerNamespace, managerAppName string) *networkingv1.NetworkPolicy {
	if managerAppName == "" {
		managerAppName = ManagerAppNameLabel
	}
	grpc := GRPCPort(env)
	proto := corev1.ProtocolTCP
	port := intstr.FromInt32(grpc)
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      NetworkPolicyName(env),
			Namespace: env.Namespace,
			Labels:    Labels(env),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: Labels(env)},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				Ports: []networkingv1.NetworkPolicyPort{{
					Protocol: &proto,
					Port:     &port,
				}},
				From: []networkingv1.NetworkPolicyPeer{{
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{NamespaceNameLabel: managerNamespace},
					},
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{LabelK8sName: managerAppName},
					},
				}},
			}},
		},
	}
}
