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

package v1alpha1

// Condition types shared by Environment and Share resources.
const (
	ConditionReady            = "Ready"
	ConditionEnabled          = "Enabled"
	ConditionAgentReady       = "AgentReady"
	ConditionEnvironmentReady = "EnvironmentReady"
	ConditionShareCreated     = "ShareCreated"
	ConditionNameReady        = "NameReady"
)

// Finalizer names.
const (
	EnvironmentFinalizer = "zrok.k8s.zrok.io/environment"
	ShareFinalizer       = "zrok.k8s.zrok.io/share"
	AccessFinalizer      = "zrok.k8s.zrok.io/access"
)

// DefaultEnableTokenKey is the Secret key for the zrok account enable token.
const DefaultEnableTokenKey = "enable-token"

// DefaultNamespaceToken is the default zrok namespace token (v2).
const DefaultNamespaceToken = "public"

// ReclaimPolicy controls remote resource cleanup on CR deletion.
type ReclaimPolicy string

const (
	ReclaimDelete ReclaimPolicy = "Delete"
	ReclaimRetain ReclaimPolicy = "Retain"
)

// ShareMode is the zrok share mode.
// +kubebuilder:validation:Enum=public;private
type ShareMode string

const (
	ShareModePublic  ShareMode = "public"
	ShareModePrivate ShareMode = "private"
)

// Reservation kinds reported on ZrokShare status.
const (
	ReservationEphemeral = "ephemeral"
	ReservationReserved  = "reserved"
	ReservationPrivate   = "private"
)

// BackendMode is the zrok backend mode.
// +kubebuilder:validation:Enum=proxy;web;caddy;drive;tcpTunnel;udpTunnel;socks
type BackendMode string

const (
	BackendModeProxy     BackendMode = "proxy"
	BackendModeWeb       BackendMode = "web"
	BackendModeCaddy     BackendMode = "caddy"
	BackendModeDrive     BackendMode = "drive"
	BackendModeTCPTunnel BackendMode = "tcpTunnel"
	BackendModeUDPTunnel BackendMode = "udpTunnel"
	BackendModeSocks     BackendMode = "socks"
)
