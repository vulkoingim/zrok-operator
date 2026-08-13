package status

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSetCondition(t *testing.T) {
	var conditions []metav1.Condition
	SetCondition(&conditions, "Ready", metav1.ConditionFalse, "Waiting", "not yet", 1)
	if len(conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(conditions))
	}
	firstTransition := conditions[0].LastTransitionTime

	SetCondition(&conditions, "Ready", metav1.ConditionFalse, "Waiting", "not yet", 1)
	if !conditions[0].LastTransitionTime.Equal(&firstTransition) {
		t.Fatal("expected no-op to preserve LastTransitionTime")
	}

	SetCondition(&conditions, "Ready", metav1.ConditionTrue, "Ready", "ok", 2)
	if len(conditions) != 1 {
		t.Fatalf("expected upsert, got %d", len(conditions))
	}
	if !IsTrue(conditions, "Ready") {
		t.Fatal("expected Ready=True")
	}
	if Reason(conditions, "Ready") != "Ready" {
		t.Fatal("expected reason Ready")
	}
	if conditions[0].LastTransitionTime.Equal(&firstTransition) {
		t.Fatal("expected transition time to change on status flip")
	}
}
