package status

import (
	"context"

	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PatchStatus re-gets obj, runs mutate, then status-patches with MergeFrom + conflict retry.
// All status field writes for that reconcile step must happen inside mutate.
func PatchStatus(ctx context.Context, c client.Client, obj client.Object, mutate func() error) error {
	key := client.ObjectKeyFromObject(obj)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := c.Get(ctx, key, obj); err != nil {
			return err
		}
		before := obj.DeepCopyObject().(client.Object)
		if err := mutate(); err != nil {
			return err
		}
		return c.Status().Patch(ctx, obj, client.MergeFrom(before))
	})
}
