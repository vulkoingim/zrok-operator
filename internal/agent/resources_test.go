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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	zrokv1alpha1 "github.com/vulkoingim/zrok-operator/api/v1alpha1"
)

func TestDesiredResources(t *testing.T) {
	env := &zrokv1alpha1.ZrokEnvironment{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "demo"},
		Spec:       zrokv1alpha1.ZrokEnvironmentSpec{},
	}
	if got := DeploymentName(env); got != "default-agent" {
		t.Fatalf("deployment name: %s", got)
	}
	if got := AgentBaseURL(env); got != "http://default-agent.demo.svc:8888" {
		t.Fatalf("base url: %s", got)
	}
	dep := DesiredDeployment(env, "tok")
	if len(dep.Spec.Template.Spec.InitContainers) != 2 {
		t.Fatalf("expected 2 init containers")
	}
	if dep.Spec.Template.Spec.Containers[0].Name != AppName {
		t.Fatalf("unexpected container")
	}
}
