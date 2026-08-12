package agent

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	zrokv1alpha1 "github.com/vulkoingim/zrok-operator/api/v1alpha1"
)

const (
	// DefaultImage is the pinned zrok2 agent image.
	DefaultImage = "docker.io/openziti/zrok2:2.0.4"

	// DefaultSocatImage proxies TCP→unix for native agent gRPC.
	DefaultSocatImage = "docker.io/alpine/socat:1.8.1.3"

	// ZrokUID is the non-root user in the openziti/zrok2 image.
	ZrokUID int64 = 2171

	// AppName is the agent app label / container name.
	AppName = "zrok-agent"

	// seedVolumeName mounts the identity Secret into the seed init container.
	seedVolumeName = "zrok-seed"

	// DefaultGRPCPort is the TCP port that proxies to agent.socket.
	DefaultGRPCPort int32 = 7777

	homeMountPath  = "/mnt"
	agentSocket    = homeMountPath + "/.zrok2/agent.socket"
	pvcNameSuffix  = "-zrok-home"
	deploySuffix   = "-agent"
	svcSuffix      = "-agent"
	idSecretSuffix = "-zrok-identity"
)

// IdentitySecretName returns the Secret that seeds agent ~/.zrok2.
func IdentitySecretName(env *zrokv1alpha1.ZrokEnvironment) string {
	return env.Name + idSecretSuffix
}

// EnvironmentDescription is the remote zrok environment description (for UI / matching).
func EnvironmentDescription(env *zrokv1alpha1.ZrokEnvironment) string {
	return fmt.Sprintf("zrok-operator/%s/%s", env.Namespace, env.Name)
}

// ManagedFrontendName is the recommended reserved-name convention for operator-managed
// public shares. Visible in the zrok UI as the frontend subdomain.
// Example: ko-default-nginx → https://ko-default-nginx.shares.zrok.io
func ManagedFrontendName(share *zrokv1alpha1.ZrokShare) string {
	return fmt.Sprintf("ko-%s-%s", share.Namespace, share.Name)
}

// Labels returns standard labels for agent resources owned by env.
func Labels(env *zrokv1alpha1.ZrokEnvironment) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       AppName,
		"app.kubernetes.io/instance":   env.Name,
		"app.kubernetes.io/component":  "agent",
		"app.kubernetes.io/managed-by": "zrok-operator",
		"zrok.k8s.zrok.io/environment": env.Name,
	}
}

// PVCName returns the PVC name for an environment.
func PVCName(env *zrokv1alpha1.ZrokEnvironment) string {
	return env.Name + pvcNameSuffix
}

// DeploymentName returns the agent Deployment name.
func DeploymentName(env *zrokv1alpha1.ZrokEnvironment) string {
	return env.Name + deploySuffix
}

// ServiceName returns the agent Service name.
func ServiceName(env *zrokv1alpha1.ZrokEnvironment) string {
	return env.Name + svcSuffix
}

// ConsolePort returns the configured console port or default.
func ConsolePort(env *zrokv1alpha1.ZrokEnvironment) int32 {
	if env.Spec.Agent.ConsolePort > 0 {
		return env.Spec.Agent.ConsolePort
	}
	return 8888
}

// GRPCPort returns the TCP port that proxies to the agent unix socket.
func GRPCPort(_ *zrokv1alpha1.ZrokEnvironment) int32 {
	return DefaultGRPCPort
}

// AgentDialAddr returns host:port for native agent gRPC (TCP→unix proxy).
func AgentDialAddr(env *zrokv1alpha1.ZrokEnvironment) string {
	return fmt.Sprintf("%s.%s.svc:%d", ServiceName(env), env.Namespace, GRPCPort(env))
}

// AgentBaseURL is an alias for AgentDialAddr (kept for call-site stability).
func AgentBaseURL(env *zrokv1alpha1.ZrokEnvironment) string {
	return AgentDialAddr(env)
}

// Image returns the agent container image.
func Image(env *zrokv1alpha1.ZrokEnvironment) string {
	if env.Spec.Agent.Image != "" {
		return env.Spec.Agent.Image
	}
	return DefaultImage
}

// DesiredPVC builds the PVC for agent HOME persistence.
func DesiredPVC(env *zrokv1alpha1.ZrokEnvironment) *corev1.PersistentVolumeClaim {
	size := resource.MustParse("1Gi")
	if !env.Spec.Agent.Persistence.Size.IsZero() {
		size = env.Spec.Agent.Persistence.Size
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PVCName(env),
			Namespace: env.Namespace,
			Labels:    Labels(env),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: size,
				},
			},
			StorageClassName: env.Spec.Agent.Persistence.StorageClassName,
		},
	}
	return pvc
}

// DesiredService builds the agent Service (HTTP console + gRPC proxy).
func DesiredService(env *zrokv1alpha1.ZrokEnvironment) *corev1.Service {
	consolePort := ConsolePort(env)
	grpcPort := GRPCPort(env)
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceName(env),
			Namespace: env.Namespace,
			Labels:    Labels(env),
		},
		Spec: corev1.ServiceSpec{
			Selector: Labels(env),
			Ports: []corev1.ServicePort{
				{
					Name:       "console",
					Port:       consolePort,
					TargetPort: intstr.FromInt32(consolePort),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "grpc",
					Port:       grpcPort,
					TargetPort: intstr.FromInt32(grpcPort),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
}

// DesiredDeployment builds the agent Deployment (init: seed identity, main: agent + grpc proxy).
func DesiredDeployment(env *zrokv1alpha1.ZrokEnvironment) *appsv1.Deployment {
	replicas := int32(1)
	if env.Spec.Agent.Replicas != nil {
		replicas = *env.Spec.Agent.Replicas
	}
	port := ConsolePort(env)
	image := Image(env)
	labels := Labels(env)

	resources := env.Spec.Agent.Resources
	if resources.Requests == nil {
		resources.Requests = corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("50m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		}
	}
	if resources.Limits == nil {
		resources.Limits = corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		}
	}

	runAsUser := ZrokUID
	runAsNonRoot := true
	allowPrivEsc := false
	readOnlyRoot := false

	secCtx := &corev1.SecurityContext{
		RunAsUser:                &runAsUser,
		RunAsGroup:               &runAsUser,
		RunAsNonRoot:             &runAsNonRoot,
		AllowPrivilegeEscalation: &allowPrivEsc,
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
		ReadOnlyRootFilesystem: &readOnlyRoot,
	}

	podSec := &corev1.PodSecurityContext{
		RunAsUser:    &runAsUser,
		RunAsGroup:   &runAsUser,
		FSGroup:      &runAsUser,
		RunAsNonRoot: &runAsNonRoot,
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}

	volMount := corev1.VolumeMount{Name: "zrok-home", MountPath: homeMountPath}
	seedMount := corev1.VolumeMount{Name: seedVolumeName, MountPath: "/seed", ReadOnly: true}
	portStr := fmt.Sprintf("%d", port)
	grpcPort := GRPCPort(env)
	// PVC ownership comes from PodSecurityContext.FSGroup (2171). Do not run a root
	// chown init — pod runAsNonRoot=true rejects UID 0 (CreateContainerConfigError).
	// Enable is done by the manager (stores EnvZID); init only seeds ~/.zrok2 from Secret.

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DeploymentName(env),
			Namespace: env.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					SecurityContext: podSec,
					InitContainers: []corev1.Container{
						{
							Name:            seedVolumeName,
							Image:           image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command: []string{"bash", "-c", strings.Join([]string{
								`set -euo pipefail`,
								`mkdir -p /mnt/.zrok2/identities`,
								`# Seed once for environment.json; always repair missing identity file.`,
								`if [[ ! -s /mnt/.zrok2/environment.json ]]; then`,
								`  cp /seed/metadata.json /mnt/.zrok2/metadata.json`,
								`  cp /seed/config.json /mnt/.zrok2/config.json`,
								`  cp /seed/environment.json /mnt/.zrok2/environment.json`,
								`  chmod 600 /mnt/.zrok2/environment.json`,
								`  echo "INFO: seeded environment.json from Secret"`,
								`fi`,
								`# zrok expects identities/environment.json (EnvironmentIdentityName + .json)`,
								`if [[ ! -s /mnt/.zrok2/identities/environment.json ]]; then`,
								`  cp /seed/identity /mnt/.zrok2/identities/environment.json`,
								`  chmod 600 /mnt/.zrok2/identities/environment.json`,
								`  rm -f /mnt/.zrok2/identities/environment`,
								`  echo "INFO: seeded identities/environment.json from Secret"`,
								`fi`,
							}, "\n")},
							SecurityContext: secCtx,
							VolumeMounts:    []corev1.VolumeMount{volMount, seedMount},
						},
					},
					Containers: []corev1.Container{
						{
							Name:            AppName,
							Image:           image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command: []string{
								"bash",
								"-c",
								// Operator owns share lifecycle: wipe agent registry so ReloadRegistry
								// does not race SharePublic against live remote reserved names (409 loop).
								`rm -f /mnt/.zrok2/agent.socket /mnt/.zrok2/agent-registry.json && exec zrok2 agent start --console-address 0.0.0.0 --console-start-port "$PORT" --console-end-port "$PORT"`,
							},
							Env: []corev1.EnvVar{
								{Name: "HOME", Value: homeMountPath},
								{Name: "PORT", Value: portStr},
							},
							Ports: []corev1.ContainerPort{{
								Name:          "console",
								ContainerPort: port,
								Protocol:      corev1.ProtocolTCP,
							}},
							Resources:       resources,
							SecurityContext: secCtx,
							VolumeMounts:    []corev1.VolumeMount{volMount},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/v1/agent/version",
										Port: intstr.FromInt32(port),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       10,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/v1/agent/version",
										Port: intstr.FromInt32(port),
									},
								},
								InitialDelaySeconds: 15,
								PeriodSeconds:       20,
							},
						},
						{
							// Agent gRPC is unix-socket-only; proxy it to TCP for the manager.
							Name:            "grpc-proxy",
							Image:           DefaultSocatImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command: []string{
								"sh", "-c",
								fmt.Sprintf(
									`while [ ! -S %s ]; do sleep 1; done; exec socat TCP-LISTEN:%d,fork,reuseaddr,bind=0.0.0.0 UNIX-CONNECT:%s`,
									agentSocket, grpcPort, agentSocket,
								),
							},
							Ports: []corev1.ContainerPort{{
								Name:          "grpc",
								ContainerPort: grpcPort,
								Protocol:      corev1.ProtocolTCP,
							}},
							SecurityContext: secCtx,
							VolumeMounts:    []corev1.VolumeMount{volMount},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("10m"),
									corev1.ResourceMemory: resource.MustParse("16Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("64Mi"),
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "zrok-home",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: PVCName(env),
								},
							},
						},
						{
							Name: seedVolumeName,
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: IdentitySecretName(env),
								},
							},
						},
					},
				},
			},
		},
	}
}
