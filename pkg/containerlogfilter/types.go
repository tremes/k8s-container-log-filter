package containerlogfilter

import "regexp"

type LogRequestsObject struct {
	LogRequests LogRequests `json:"log_requests"`
}

type LogRequests []LogRequest

type Messages []string

type LogRequest struct {
	Namespace      string        `json:"namespace"`
	PodNameRegExpr regexp.Regexp `json:"pod_name_regex"`
	Messages       Messages      `json:"messages"`
}

type AggregatedNamespaceData struct {
	PodNameRegExpr string
	Messages       []string
}
