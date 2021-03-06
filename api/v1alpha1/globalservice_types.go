/*


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

// GlobalServiceSpec defines the desired state of GlobalService
type GlobalServiceSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Ports      []GlobalServicePort `json:"ports,omitempty"`
	ClusterSet []string            `json:"clusterSet,omitempty"`
}

type GlobalServicePort struct {
	Name     string `json:"name,omitempty"`
	Protocol string `json:"protocol,omitempty"`
	Port     int    `json:"port,omitempty"`
}

// GlobalServiceStatus defines the observed state of GlobalService
type GlobalServiceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Endpoints represents a list of endpoint for the global service.
	Endpoints []GlobalEndpoint `json:"endpoints,omitempty"`
	VIP       string           `json:"vip,omitempty"`
	State     string           `json:"state,omitempty"`
}

type GlobalEndpoint struct {
	Cluster string `json:"cluster,omitempty"`
	IP      string `json:"ip,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ports",type=string,JSONPath=`.spec.ports[*].port`
// +kubebuilder:printcolumn:name="VIP",type=string,JSONPath=`.status.vip`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`

// GlobalService is the Schema for the globalservices API
type GlobalService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GlobalServiceSpec   `json:"spec,omitempty"`
	Status GlobalServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GlobalServiceList contains a list of GlobalService
type GlobalServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GlobalService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GlobalService{}, &GlobalServiceList{})
}
