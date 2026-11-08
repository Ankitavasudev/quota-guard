// Package v1alpha1 registers the ResourceBudget API group and version.
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

const (
	// GroupName is the API group for the ResourceBudget custom resource.
	GroupName = "guard.ankaita.dev"
	// Version is the API version.
	Version = "v1alpha1"
)

// SchemeGroupVersion is the schema.GroupVersion for this package.
var SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: Version}

// SchemeBuilder is what controller-runtime registers with its scheme.
var SchemeBuilder = &scheme.Builder{GroupVersion: SchemeGroupVersion}

// GroupVersion is the schema.GroupVersion for this package (kept for compatibility with kubebuilder-style code).
var GroupVersion = SchemeGroupVersion

// AddToScheme registers the types in this package with the scheme.
func AddToScheme(s *runtime.Scheme) error {
	return SchemeBuilder.AddToScheme(s)
}

func init() {
	SchemeBuilder.Register(&ResourceBudget{}, &ResourceBudgetList{})
}
