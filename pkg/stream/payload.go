package stream

// KVPair is a single key-value entry in render output.
type KVPair struct {
	Key   string
	Value string
}

// ListItem is a single status-bearing line in render output.
type ListItem struct {
	Status string // "ok" | "warn" | "error" | "info"
	Text   string
}

// ResourceDetail carries structured information about a specific resource.
type ResourceDetail struct {
	Kind       string            `json:"kind"`
	Name       string            `json:"name"`
	Namespace  string            `json:"namespace"`
	Status     map[string]string `json:"status"`
	Containers []ContainerInfo   `json:"containers,omitempty"`
	Conditions []ConditionInfo   `json:"conditions,omitempty"`
	Events     []EventInfo       `json:"events,omitempty"`
	RecentLogs string            `json:"recent_logs,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
}

// ContainerInfo describes a container within a Pod.
type ContainerInfo struct {
	Name         string            `json:"name"`
	Image        string            `json:"image"`
	Ready        bool              `json:"ready"`
	RestartCount int32             `json:"restart_count"`
	State        string            `json:"state"`
	Resources    map[string]string `json:"resources,omitempty"`
}

// ConditionInfo describes a resource condition.
type ConditionInfo struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// EventInfo describes a Kubernetes event.
type EventInfo struct {
	Type      string `json:"type"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}
