package report

import "fmt"

func Validate(r DiagnosisReport) error {
	ids := make(map[string]struct{}, len(r.Evidence))
	for _, ev := range r.Evidence {
		ids[ev.ID] = struct{}{}
	}
	check := func(refs []string, field string) error {
		for _, id := range refs {
			if _, ok := ids[id]; !ok {
				return fmt.Errorf("%s references unknown evidence id %q", field, id)
			}
		}
		return nil
	}
	if err := check(r.RootCause.EvidenceIDs, "root_cause"); err != nil {
		return err
	}
	for i, h := range r.Hypotheses {
		if err := check(h.SupportingEvidence, fmt.Sprintf("hypothesis[%d]", i)); err != nil {
			return err
		}
	}
	if r.Verdict == VerdictConfirmed && r.RootCause.ConfidenceScore < 0.85 {
		return fmt.Errorf("confirmed verdict requires confidence >= 0.85")
	}
	if r.Verdict == VerdictConfirmed && r.RootCause.Summary == "" {
		return fmt.Errorf("confirmed verdict requires root cause summary")
	}
	if len(r.Evidence) == 0 {
		return fmt.Errorf("report requires evidence")
	}
	return nil
}
