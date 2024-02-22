/*
Copyright 2024 Flant JSC
Licensed under the Deckhouse Platform Enterprise Edition (EE) license. See https://github.com/deckhouse/deckhouse/blob/main/ee/LICENSE
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type VMType string

const (
	VMTypeHighPerformance VMType = "high_performance"
	VMTypeHighServer      VMType = "server"
	VMTypeHighDesktop     VMType = "desktop"
)

// ZvirtMachineSpec defines the desired state of ZvirtMachine
type ZvirtMachineSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ID is the UUID of the VM
	// +optional
	ID string `json:"id"`
	// ProviderID is the UUID of the VM, prefixed with 'zvirt://' proto.
	// +optional
	ProviderID string `json:"providerID,omitempty"`
	// The VM template this instance will be created from.
	TemplateName string `json:"template_name"`
	// the zVirt cluster this VM instance belongs too.
	ClusterID string `json:"cluster_id"`
	// VNICProfileID the id of the zVirt vNic profile for the VM.
	VNICProfileID string `json:"vnic_profile_id"`
	// CPU defines the VM CPU.
	// +optional
	CPU *CPU `json:"cpu,omitempty"`
	// MemoryMB is the size of a VM's memory in MiBs.
	// +optional
	Memory int32 `json:"memory,omitempty"`
	// VMType defines the workload type the instance will
	// be used for and this effects the instance parameters.
	// One of "desktop, server, high_performance"
	// +kubebuilder:validation:Enum="";high_performance;server;desktop
	// +optional
	VMType VMType `json:"vmType,omitempty"`
}

// CPU defines the VM cpu, made of (Sockets * Cores * Threads).
// Most of the time you should only set Sockets to the number of cores you want VM to have and set Cores and Threads to 1.
type CPU struct {
	// Sockets is the number of sockets for a VM.
	// +kubebuilder:default=4
	Sockets int32 `json:"sockets"`

	// Cores is the number of cores per socket.
	// +kubebuilder:default=1
	Cores int32 `json:"cores"`

	// Threads is the number of thread per core.
	// +kubebuilder:default=1
	Threads int32 `json:"threads"`
}

type Disk struct {
	// SizeGB size of the bootable disk in GiB.
	SizeGB int64 `json:"size_gb"`
}

// ZvirtMachineStatus defines the observed state of ZvirtMachine
type ZvirtMachineStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Ready indicates the VM has been provisioned and is ready.
	Ready bool `json:"ready"`

	// Addresses holds a list of the host names, external IP addresses, internal IP addresses, external DNS names, and/or internal DNS names for the VM.
	// +optional
	Addresses []VMAddress `json:"addresses,omitempty"`

	// FailureReason will contain an error type if something goes wrong during Machine lifecycle.
	// +optional
	FailureReason *string `json:"failureReason,omitempty"`

	// FailureMessage will describe an error if something goes wrong during Machine lifecycle.
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`

	// Conditions defines current service state of the StaticMachine.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

type VMAddress struct {
	Type    clusterv1.MachineAddressType `json:"type"`
	Address string                       `json:"address"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ZvirtMachine is the Schema for the zvirtmachines API
type ZvirtMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ZvirtMachineSpec   `json:"spec,omitempty"`
	Status ZvirtMachineStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ZvirtMachineList contains a list of ZvirtMachine
type ZvirtMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ZvirtMachine `json:"items"`
}

// GetConditions gets the StaticInstance status conditions
func (r *ZvirtMachine) GetConditions() clusterv1.Conditions {
	return r.Status.Conditions
}

// SetConditions sets the StaticInstance status conditions
func (r *ZvirtMachine) SetConditions(conditions clusterv1.Conditions) {
	r.Status.Conditions = conditions
}

func init() {
	objectTypes = append(objectTypes, &ZvirtMachine{}, &ZvirtMachineList{})
}
