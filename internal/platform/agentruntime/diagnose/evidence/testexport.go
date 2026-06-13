package evidence

import "github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/casefile"

// DeterministicForTest exposes deterministic extraction for unit tests.
func DeterministicForTest(obs casefile.Observation) []casefile.Evidence {
	return deterministic(obs)
}
