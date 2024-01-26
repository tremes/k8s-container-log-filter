package containerlogfilter

type LogRequestsObject struct {
	LogRequests LogRequests `json:"log_requests"`
}

type LogRequests []LogRequest

// LogRequest represents a request to filter and collect particular messages
// from particular containers.
// This is data provided by the user.
type LogRequest struct {
	Namespace      string   `json:"namespace"`
	PodNameRegExpr string   `json:"pod_name_regex"`
	Messages       []string `json:"messages"`
}
