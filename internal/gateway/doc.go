/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
