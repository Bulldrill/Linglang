package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HilbertSpaceSpec describes a quantum subspace as a Kubernetes resource
// (issue #21): its dimension, which QuantumBackend technology its pods
// should run (core.OpenQuantumBackend's technology names), and how many
// quantum-node (issue #20) replicas should back it.
type HilbertSpaceSpec struct {
	// Dim is the Hilbert space dimension (e.g. 2^n for an n-qubit register).
	// +kubebuilder:validation:Minimum=2
	Dim int32 `json:"dim"`

	// Technology selects the simulated/real backend each pod runs —
	// simulator | superconductor | trapped-ion | gpu | noisy — mirroring
	// core.OpenQuantumBackend's technology names (issue #13).
	// +kubebuilder:default=simulator
	Technology string `json:"technology,omitempty"`

	// Replicas is the desired number of quantum-node pods backing this
	// space. More than one models a replicated/load-balanced QPU pool;
	// entanglement tracking (Status.EntangledWith) is independent of pod
	// count.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	Replicas int32 `json:"replicas,omitempty"`
}

// HilbertSpacePhase is the operator's coarse-grained lifecycle state for a
// HilbertSpace, surfaced via `kubectl get hilbertspaces`.
type HilbertSpacePhase string

const (
	PhasePending      HilbertSpacePhase = "Pending"
	PhaseProvisioning HilbertSpacePhase = "Provisioning"
	PhaseReady        HilbertSpacePhase = "Ready"
)

// HilbertSpaceStatus reports the operator's observed state: pod lifecycle
// (ReadyReplicas vs. Spec.Replicas) and active entanglement — which other
// HilbertSpace resources this one currently shares a Bell pair with via a
// QuantumChannel (core.HilbertRegistry, issue #16), the "estado de
// entrelazamiento activo" the issue asks the operator to track.
type HilbertSpaceStatus struct {
	// +kubebuilder:default=Pending
	Phase HilbertSpacePhase `json:"phase,omitempty"`

	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// EntangledWith lists the names of other HilbertSpace resources this
	// one is currently entangled with (set externally, e.g. by a scheduler
	// invoking core.Teleport/ScheduleCircuit — the operator surfaces this
	// state, it does not itself run quantum protocols).
	EntangledWith []string `json:"entangledWith,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Dim",type=integer,JSONPath=`.spec.dim`
// +kubebuilder:printcolumn:name="Technology",type=string,JSONPath=`.spec.technology`
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.spec.replicas`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:resource:shortName=hspace

// HilbertSpace is the Schema for the hilbertspaces API.
type HilbertSpace struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HilbertSpaceSpec   `json:"spec,omitempty"`
	Status HilbertSpaceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// HilbertSpaceList contains a list of HilbertSpace.
type HilbertSpaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HilbertSpace `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HilbertSpace{}, &HilbertSpaceList{})
}
