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

// DefaultUniqueIDNamespace is the Namespace whose UUID (metadata.uid) is used as
// ZrokEnvironmentSpec.UniqueID when that field is empty.
const DefaultUniqueIDNamespace = "kube-system"

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
