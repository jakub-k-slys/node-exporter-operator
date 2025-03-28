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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
type NodeExporterSettings struct {
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Enabled bool `json:"enabled,omitempty"`
}

type BlackboxExporterSettings struct {
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Enabled bool `json:"enabled,omitempty"`
}

type KubeStateMetricsSettings struct {
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	Enabled bool `json:"enabled,omitempty"`
}

// NodeExporterSpec defines the desired state of NodeExporter
type NodeExporterSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +operator-sdk:csv:customresourcedefinitions:type=spec
	NodeExporterSettings NodeExporterSettings `json:"nodeExporter,omitempty"`
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	BlackboxExporterSettings BlackboxExporterSettings `json:"blackboxExporter,omitempty"`
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	KubeStateMetricsSettings KubeStateMetricsSettings `json:"kubeStateMetrics,omitempty"`
}

// NodeExporterStatus defines the observed state of NodeExporter
type NodeExporterStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// NodeExporter is the Schema for the nodeexporters API
// +kubebuilder:subresource:status
type NodeExporter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeExporterSpec   `json:"spec,omitempty"`
	Status NodeExporterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NodeExporterList contains a list of NodeExporter
type NodeExporterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeExporter `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeExporter{}, &NodeExporterList{})
}
