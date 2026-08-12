package controller

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	zrokv1alpha1 "github.com/vulkoingim/zrok-operator/api/v1alpha1"
)

// environmentRefField indexes Share/Access by spec.environmentRef.name.
const environmentRefField = ".spec.environmentRef.name"

func indexByEnvironmentRef(obj client.Object) []string {
	switch o := obj.(type) {
	case *zrokv1alpha1.ZrokShare:
		if o.Spec.EnvironmentRef.Name == "" {
			return nil
		}
		return []string{o.Spec.EnvironmentRef.Name}
	case *zrokv1alpha1.ZrokAccess:
		if o.Spec.EnvironmentRef.Name == "" {
			return nil
		}
		return []string{o.Spec.EnvironmentRef.Name}
	default:
		return nil
	}
}

func setupEnvironmentRefIndex(mgr manager.Manager, obj client.Object) error {
	return mgr.GetFieldIndexer().IndexField(context.Background(), obj, environmentRefField, indexByEnvironmentRef)
}
