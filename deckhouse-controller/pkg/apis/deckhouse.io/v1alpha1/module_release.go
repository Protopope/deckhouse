/*
Copyright 2023 Flant JSC

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

package v1alpha1

import (
	"time"

	"github.com/Masterminds/semver/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	PhasePending         = "Pending"
	PhasePolicyUndefined = "PolicyUndefined"
	PhaseDeployed        = "Deployed"
	PhaseSuperseded      = "Superseded"
	PhaseSuspended       = "Suspended"
)

var (
	ModuleReleaseGVR = schema.GroupVersionResource{
		Group:    SchemeGroupVersion.Group,
		Version:  SchemeGroupVersion.Version,
		Resource: "modulereleases",
	}
	ModuleReleaseGVK = schema.GroupVersionKind{
		Group:   SchemeGroupVersion.Group,
		Version: SchemeGroupVersion.Version,
		Kind:    "ModuleRelease",
	}
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ModuleRelease is a Module release object.
type ModuleRelease struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ModuleReleaseSpec `json:"spec"`

	Status ModuleReleaseStatus `json:"status,omitempty"`
}

func (mr *ModuleRelease) GetVersion() *semver.Version {
	return mr.Spec.Version
}

func (mr *ModuleRelease) GetName() string {
	return mr.Name
}

func (mr *ModuleRelease) GetApplyAfter() *time.Time {
	return &mr.Spec.ApplyAfter.Time
}

func (mr *ModuleRelease) GetRequirements() map[string]string {
	return mr.Spec.Requirements
}

func (mr *ModuleRelease) GetChangelogLink() string {
	return ""
}

func (mr *ModuleRelease) GetCooldownUntil() *time.Time {
	return nil
}

func (mr *ModuleRelease) GetApproved() bool {
	//TODO implement me
	panic("implement me")
}

func (mr *ModuleRelease) GetDisruptions() []string {
	//TODO implement me
	panic("implement me")
}

func (mr *ModuleRelease) GetDisruptionApproved() bool {
	//TODO implement me
	panic("implement me")
}

func (mr *ModuleRelease) GetPhase() string {
	//TODO implement me
	panic("implement me")
}

func (mr *ModuleRelease) GetForce() bool {
	//TODO implement me
	panic("implement me")
}

func (mr *ModuleRelease) GetApplyNow() bool {
	//TODO implement me
	panic("implement me")
}

func (mr *ModuleRelease) SetApprovedStatus(b bool) {
	//TODO implement me
	panic("implement me")
}

func (mr *ModuleRelease) GetSuspend() bool {
	//TODO implement me
	panic("implement me")
}

func (mr *ModuleRelease) GetManuallyApproved() bool {
	//TODO implement me
	panic("implement me")
}

func (mr *ModuleRelease) GetApprovedStatus() bool {
	//TODO implement me
	panic("implement me")
}

func (mr *ModuleRelease) GetMessage() string {
	//TODO implement me
	panic("implement me")
}

// GetModuleSource returns module source for this release
func (mr *ModuleRelease) GetModuleSource() string {
	for _, ref := range mr.GetOwnerReferences() {
		if ref.APIVersion == ModuleSourceGVK.GroupVersion().String() && ref.Kind == ModuleSourceGVK.Kind {
			return ref.Name
		}
	}

	return mr.Labels["source"]
}

type ModuleReleaseSpec struct {
	ModuleName string          `json:"moduleName"`
	Version    *semver.Version `json:"version,omitempty"`
	Weight     uint32          `json:"weight,omitempty"`

	ApplyAfter   *metav1.Time      `json:"applyAfter,omitempty"`
	Requirements map[string]string `json:"requirements,omitempty"`
}

type ModuleReleaseStatus struct {
	Phase          string      `json:"phase,omitempty"`
	Approved       bool        `json:"approved"`
	TransitionTime metav1.Time `json:"transitionTime,omitempty"`
	Message        string      `json:"message"`
}

type moduleReleaseKind struct{}

func (in *ModuleReleaseStatus) GetObjectKind() schema.ObjectKind {
	return &moduleReleaseKind{}
}

func (f *moduleReleaseKind) SetGroupVersionKind(_ schema.GroupVersionKind) {}
func (f *moduleReleaseKind) GroupVersionKind() schema.GroupVersionKind {
	return ModuleReleaseGVK
}

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ModuleReleaseList is a list of ModuleRelease resources
type ModuleReleaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []ModuleRelease `json:"items"`
}
