/*
Copyright 2022.

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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

//ServiceType defines the Service in Kubernetes of ShardingSphere-Proxy
type ServiceType struct {
	Type v1.ServiceType `json:"type"`
	// +optional
	NodePort int32 `json:"nodePort"`
}
type MySQLDriver struct {
	Version string `json:"version"`
}

// ProxySpec defines the desired state of Proxy
type ProxySpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// TODO:description
	Version     string      `json:"version"`
	ServiceType ServiceType `json:"serviceType"`
	// TODO:description
	Replicas int32 `json:"replicas"`
	// TODO:description
	ProxyConfigName string `json:"proxyConfigName"`
	// TODO:description
	Port int32 `json:"port"`
	// TODO:description
	// +optional
	MySQLDriver *MySQLDriver `json:"mySQLDriver,omitempty"`
	// +optional
	Resources *v1.ResourceRequirements `json:"resources,omitempty"`
	// Probes are not allowed for ephemeral containers.
	// +optional
	LivenessProbe *v1.Probe `json:"livenessProbe,omitempty"`
	// Probes are not allowed for ephemeral containers.
	// +optional
	ReadinessProbe *v1.Probe `json:"readinessProbe,omitempty" `
	// Probes are not allowed for ephemeral containers.
	// +optional
	StartupProbe *v1.Probe `json:"startupProbe,omitempty"`
}

//+kubebuilder:printcolumn:JSONPath=".status.availableNodes",name=AvailableNodes,type=string
//+kubebuilder:printcolumn:JSONPath=".status.version",name=Version,type=string
//+kubebuilder:printcolumn:JSONPath=".status.phase",name=Phase,type=string
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Proxy is the Schema for the proxies API
type Proxy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProxySpec   `json:"spec,omitempty"`
	Status ProxyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ProxyList contains a list of Proxy
type ProxyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Proxy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Proxy{}, &ProxyList{})
}
