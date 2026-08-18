package e2e

import (
	"fmt"
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"

	zrokv1alpha1 "github.com/vulkoingim/zrok-operator/api/v1alpha1"
	"github.com/vulkoingim/zrok-operator/test/utils"
)

type environmentShareFixture struct {
	Secret *corev1.Secret
	Env    *zrokv1alpha1.ZrokEnvironment
	Share  *zrokv1alpha1.ZrokShare
}

func loadEnvironmentShareFixture(enableToken, reservedName string) (*environmentShareFixture, error) {
	root, err := utils.GetProjectDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(root, "config", "samples", "zrok_v1alpha1_environment_share.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read sample %s: %w", path, err)
	}

	objects, err := decodeYAMLDocuments(data)
	if err != nil {
		return nil, err
	}

	fixture := &environmentShareFixture{}
	for _, obj := range objects {
		switch typed := obj.(type) {
		case *corev1.Secret:
			fixture.Secret = typed.DeepCopy()
		case *zrokv1alpha1.ZrokEnvironment:
			fixture.Env = typed.DeepCopy()
		case *zrokv1alpha1.ZrokShare:
			fixture.Share = typed.DeepCopy()
		default:
			return nil, fmt.Errorf("unexpected sample type %T", obj)
		}
	}
	if fixture.Secret == nil || fixture.Env == nil || fixture.Share == nil {
		return nil, fmt.Errorf("sample missing Secret, ZrokEnvironment, or ZrokShare")
	}

	if fixture.Secret.StringData == nil {
		fixture.Secret.StringData = map[string]string{}
	}
	fixture.Secret.StringData[zrokv1alpha1.DefaultEnableTokenKey] = enableToken

	if fixture.Share.Spec.NameSelection == nil {
		fixture.Share.Spec.NameSelection = &zrokv1alpha1.NameSelectionSpec{}
	}
	fixture.Share.Spec.NameSelection.Name = reservedName

	return fixture, nil
}
