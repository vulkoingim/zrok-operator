package status

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SetCondition upserts a condition into the slice.
// No-op when type/status/reason/message/observedGeneration are unchanged.
func SetCondition(conditions *[]metav1.Condition, conditionType string, status metav1.ConditionStatus, reason, message string, observedGeneration int64) {
	message = truncate(message, 1024)

	for i := range *conditions {
		if (*conditions)[i].Type != conditionType {
			continue
		}
		cur := &(*conditions)[i]
		if cur.Status == status &&
			cur.Reason == reason &&
			cur.Message == message &&
			cur.ObservedGeneration == observedGeneration {
			return
		}
		now := metav1.Now()
		if cur.Status == status {
			now = cur.LastTransitionTime
		}
		*cur = metav1.Condition{
			Type:               conditionType,
			Status:             status,
			Reason:             reason,
			Message:            message,
			LastTransitionTime: now,
			ObservedGeneration: observedGeneration,
		}
		return
	}

	*conditions = append(*conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: observedGeneration,
	})
}

// IsTrue reports whether a condition type is True.
func IsTrue(conditions []metav1.Condition, conditionType string) bool {
	for _, c := range conditions {
		if c.Type == conditionType {
			return c.Status == metav1.ConditionTrue
		}
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
