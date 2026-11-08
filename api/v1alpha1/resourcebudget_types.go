// Package v1alpha1 contains the ResourceBudget API types.
//
// ResourceBudget lets a namespace declare a total CPU/memory request budget.
// The controller sums pod requests in the namespace and reports how the
// namespace is tracking against that budget via status conditions and events.
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:generate=true
// +kubebuilder:subresource:status

// ResourceBudget declares the maximum aggregate CPU and memory requests
// allowed inside a namespace, plus the observed usage.
type ResourceBudget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ResourceBudgetSpec   `json:"spec,omitempty"`
	Status ResourceBudgetStatus `json:"status,omitempty"`
}

// ResourceBudgetSpec is the desired budget.
type ResourceBudgetSpec struct {
	// CPU is the aggregate CPU request capacity for the namespace, e.g. "4" or "500m".
	// +kubebuilder:validation:Required
	CPU string `json:"cpu"`

	// Memory is the aggregate memory request capacity, e.g. "8Gi".
	// +kubebuilder:validation:Required
	Memory string `json:"memory"`
}

// ResourceBudgetStatus is the observed state.
type ResourceBudgetStatus struct {
	// ObservedCPU is the sum of pod CPU requests currently scheduled in the namespace.
	ObservedCPU string `json:"observedCpu,omitempty"`

	// ObservedMemory is the sum of pod memory requests currently scheduled in the namespace.
	ObservedMemory string `json:"observedMemory,omitempty"`

	// WithinBudget is true when every pod fits inside the declared budget.
	WithinBudget bool `json:"withinBudget"`

	// Pods is the number of pods counted in this calculation.
	Pods int `json:"pods"`

	// Conditions contains the latest enforcement state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastCalculated is the time this status was computed.
	LastCalculated *metav1.Time `json:"lastCalculated,omitempty"`
}

// ResourceBudgetList is a list of ResourceBudget resources.
type ResourceBudgetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ResourceBudget `json:"items"`
}
