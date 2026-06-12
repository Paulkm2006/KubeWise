package casefile

import corev1 "k8s.io/api/core/v1"

type Observation struct {
	Pod    *corev1.Pod
	Events []corev1.Event
	Logs   map[string]string
}

type Evidence struct {
	ID         string
	Type       string
	Source     string
	Signal     string
	Strength   string
	Title      string
	Summary    string
	Detail     string
	RawExcerpt string
	Refs       []string
}

const (
	TypeOOMKilled        = "oom_killed"
	TypeImagePullBackOff = "image_pull_backoff"
	TypeCrashLoopBackOff = "crashloop_backoff"
	TypeFailedScheduling = "failed_scheduling"
	TypeFailedMount      = "failed_mount"
	TypeProbeFailure     = "probe_failure"
	TypeLogSignal        = "log_signal"

	StrengthStrong   = "strong"
	StrengthModerate = "moderate"
	StrengthWeak     = "weak"
)
