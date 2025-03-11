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

package agent

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	zrokv1alpha1 "github.com/vulkoingim/zrok-operator/api/v1alpha1"
)

const (
	// DefaultImage is the pinned zrok2 agent image.
	DefaultImage = "docker.io/openziti/zrok2:v2.0.4"

	// ZrokUID is the non-root user in the openziti/zrok2 image.
	ZrokUID int64 = 2171

	// AppName is the agent app label / container name.
	AppName = "zrok-agent"

	homeMountPath = "/mnt"
	pvcNameSuffix = "-zrok-home"
	deploySuffix  = "-agent"
	svcSuffix     = "-agent"
)

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

// AgentBaseURL returns the in-cluster HTTP base URL for the agent console.
func AgentBaseURL(env *zrokv1alpha1.ZrokEnvironment) string {
	return fmt.Sprintf("http://%s.%s.svc:%d", ServiceName(env), env.Namespace, ConsolePort(env))
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

// DesiredService builds the agent Service exposing the HTTP console.
func DesiredService(env *zrokv1alpha1.ZrokEnvironment) *corev1.Service {
	port := ConsolePort(env)
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceName(env),
			Namespace: env.Namespace,
			Labels:    Labels(env),
		},
		Spec: corev1.ServiceSpec{
			Selector: Labels(env),
			Ports: []corev1.ServicePort{{
				Name:       "console",
				Port:       port,
				TargetPort: intstr.FromInt32(port),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}
}

// DesiredDeployment builds the agent Deployment (init: chown + enable, main: agent start).
func DesiredDeployment(env *zrokv1alpha1.ZrokEnvironment, enableToken string) *appsv1.Deployment {
	replicas := int32(1)
	if env.Spec.Agent.Replicas != nil {
		replicas = *env.Spec.Agent.Replicas
	}
	port := ConsolePort(env)
	image := Image(env)
	apiEndpoint := env.Spec.ApiEndpoint
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

	enableEnv := []corev1.EnvVar{
		{Name: "HOME", Value: homeMountPath},
		{Name: "ZROK2_ENABLE_TOKEN", Value: enableToken},
	}
	if apiEndpoint != "" {
		enableEnv = append(enableEnv, corev1.EnvVar{Name: "ZROK2_API_ENDPOINT", Value: apiEndpoint})
	}

	volMount := corev1.VolumeMount{Name: "zrok-home", MountPath: homeMountPath}
	portStr := fmt.Sprintf("%d", port)
	rootUser := int64(0)
	initNoPrivEsc := false

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
							Name:            "zrok-init",
							Image:           "docker.io/library/busybox:1.36",
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"sh", "-c", "chown -Rc 2171:2171 /mnt/"},
							SecurityContext: &corev1.SecurityContext{
								RunAsUser:                &rootUser,
								AllowPrivilegeEscalation: &initNoPrivEsc,
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
									Add:  []corev1.Capability{"CHOWN"},
								},
							},
							VolumeMounts: []corev1.VolumeMount{volMount},
						},
						{
							Name:            "zrok-enable",
							Image:           image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"zrok2-enable"},
							Env:             enableEnv,
							SecurityContext: secCtx,
							VolumeMounts:    []corev1.VolumeMount{volMount},
						},
					},
					Containers: []corev1.Container{{
						Name:            AppName,
						Image:           image,
						ImagePullPolicy: corev1.PullIfNotPresent,
						Command: []string{
							"bash", "-c",
							`rm -f /mnt/.zrok2/agent.socket && exec zrok2 agent start --console-address 0.0.0.0 --console-start-port "$PORT" --console-end-port "$PORT"`,
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
					}},
					Volumes: []corev1.Volume{{
						Name: "zrok-home",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: PVCName(env),
							},
						},
					}},
				},
			},
		},
	}
}
