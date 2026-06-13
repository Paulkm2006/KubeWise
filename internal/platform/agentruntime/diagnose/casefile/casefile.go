package casefile

import "time"

type Target struct {
	Cluster   string `json:"cluster"`
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
}

type MissingData struct {
	Key    string `json:"key"`
	Reason string `json:"reason"`
}

type CaseFile struct {
	QueryID     string        `json:"query_id"`
	Target      Target        `json:"target"`
	Profile     string        `json:"profile"`
	StartedAt   time.Time     `json:"started_at"`
	Catalog     *Catalog      `json:"catalog"`
	MissingData []MissingData `json:"missing_data,omitempty"`
	Observation Observation   `json:"-"`
}

func New(queryID string, target Target, profile string) *CaseFile {
	return &CaseFile{
		QueryID:   queryID,
		Target:    target,
		Profile:   profile,
		StartedAt: time.Now().UTC(),
		Catalog:   NewCatalog(),
	}
}

func (c *CaseFile) AddMissing(key, reason string) {
	if c == nil || key == "" {
		return
	}
	c.MissingData = append(c.MissingData, MissingData{Key: key, Reason: reason})
}
