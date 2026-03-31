package runner

// JobResult represents the execution result of a single job.
type JobResult struct {
	Status  string            `json:"status"`
	Outputs map[string]string `json:"outputs,omitempty"`
}

// Result represents the execution result of a workflow.
type Result struct {
	Workflow string                `json:"workflow"`
	Status   string               `json:"status"`
	Jobs     map[string]*JobResult `json:"jobs"`
	Outputs  map[string]string     `json:"outputs,omitempty"`
}
