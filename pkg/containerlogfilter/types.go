package containerlogfilter

import "k8s.io/apimachinery/pkg/util/sets"

type LogRequestsObject struct {
	LogRequests LogRequestDefinitions `json:"log_requests"`
}

type LogRequestDefinitions []LogRequestDefinition

// LogRequestDefinition represents a request to filter and collect particular messages
// from particular containers.
// This is data provided by the user.
type LogRequestDefinition struct {
	Namespace    string   `json:"namespace"`
	PodNameRegex string   `json:"pod_name_regex"`
	Messages     []string `json:"messages"`
}

type LogRequest struct {
	Namespace    string
	PodNameRegex sets.Set[string]
	Messages     sets.Set[string]
}

type ContainerLogRequest struct {
	Namespace     string
	PodName       string
	ContainerName string
	Messages      sets.Set[string]
}
