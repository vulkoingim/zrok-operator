// Package gateway reserves space for future Gateway API (HTTPRoute) translation.
// Ingress translation ships in internal/controller/ingress_controller.go.
package gateway

const (
	AnnotationEnvironment = "zrok.k8s.zrok.io/environment"
	AnnotationName        = "zrok.k8s.zrok.io/name"
	// AnnotationNamespace is the Ingress/Gateway annotation key for the zrok namespace selector.
	AnnotationNamespace = "zrok.k8s.zrok.io/namespace-" + "token"
	GatewayClassName    = "zrok"
)

// AnnotationNamespaceToken is an alias used by docs/samples.
const AnnotationNamespaceToken = AnnotationNamespace
