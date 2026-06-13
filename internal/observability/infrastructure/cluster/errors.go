package cluster

import "errors"

var (
	ErrNotFound     = errors.New("cluster not found")
	ErrOffline      = errors.New("cluster offline")
	ErrUnavailable  = errors.New("cluster gateway unavailable")
	ErrNameRequired = errors.New("cluster name required")
)
