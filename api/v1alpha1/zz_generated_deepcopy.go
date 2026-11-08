// Hand-written deepcopy helpers for the ResourceBudget API types.
// (kubebuilder would normally generate these; keeping them in-repo means the
// project builds with nothing but go.)

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyObject returns a deep copy of the object.
func (in *ResourceBudget) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}

// DeepCopy creates a deep copy of the ResourceBudget.
func (in *ResourceBudget) DeepCopy() *ResourceBudget {
	if in == nil {
		return nil
	}
	out := new(ResourceBudget)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *ResourceBudget) DeepCopyInto(out *ResourceBudget) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	out.ObjectMeta = *in.ObjectMeta.DeepCopy()
	out.Spec = in.Spec
	out.Status = *in.Status.DeepCopy()
}

// DeepCopy creates a deep copy of the ResourceBudgetStatus.
func (in *ResourceBudgetStatus) DeepCopy() *ResourceBudgetStatus {
	if in == nil {
		return nil
	}
	out := new(ResourceBudgetStatus)
	*out = *in
	if len(in.Conditions) > 0 {
		conds := make([]metav1.Condition, len(in.Conditions))
		copy(conds, in.Conditions)
		out.Conditions = conds
	}
	if in.LastCalculated != nil {
		t := *in.LastCalculated
		out.LastCalculated = &t
	}
	return out
}

// DeepCopyObject returns a deep copy of the ResourceBudgetList.
func (in *ResourceBudgetList) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}

// DeepCopy creates a deep copy of the ResourceBudgetList.
func (in *ResourceBudgetList) DeepCopy() *ResourceBudgetList {
	if in == nil {
		return nil
	}
	out := new(ResourceBudgetList)
	*out = *in
	out.TypeMeta = in.TypeMeta
	out.ListMeta = in.ListMeta
	items := make([]ResourceBudget, len(in.Items))
	for i := range in.Items {
		in.Items[i].DeepCopyInto(&items[i])
	}
	out.Items = items
	return out
}
