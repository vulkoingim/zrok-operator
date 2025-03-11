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

package status

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SetCondition upserts a condition into the slice.
func SetCondition(conditions *[]metav1.Condition, conditionType string, status metav1.ConditionStatus, reason, message string, observedGeneration int64) {
	now := metav1.Now()
	newCond := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            truncate(message, 1024),
		LastTransitionTime: now,
		ObservedGeneration: observedGeneration,
	}

	for i := range *conditions {
		if (*conditions)[i].Type != conditionType {
			continue
		}
		if (*conditions)[i].Status == status {
			newCond.LastTransitionTime = (*conditions)[i].LastTransitionTime
		}
		(*conditions)[i] = newCond
		return
	}
	*conditions = append(*conditions, newCond)
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
