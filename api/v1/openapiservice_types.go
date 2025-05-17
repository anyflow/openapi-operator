package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type OpenapiServiceSpec struct {
	Selector Selector `json:"selector"`
	Prefix   string   `json:"prefix,omitempty"`
	OpenAPI  OpenAPISpec `json:"openapi"`
}

type Selector struct {
	MatchLabels map[string]string `json:"matchLabels,omitempty"`
}

type OpenAPISpec struct {
	Paths map[string]PathItem `json:"paths,omitempty"`
}

type PathItem struct{}

type OpenapiServiceStatus struct {
	// WasmPluginName is the name of the created WasmPlugin resource
	// Format: path-template-filter-{name}
	WasmPluginName string `json:"wasmPluginName,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// OpenapiService is the Schema for the openapiservices API.
type OpenapiService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpenapiServiceSpec   `json:"spec,omitempty"`
	Status OpenapiServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OpenapiServiceList contains a list of OpenapiService.
type OpenapiServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenapiService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenapiService{}, &OpenapiServiceList{})
}
