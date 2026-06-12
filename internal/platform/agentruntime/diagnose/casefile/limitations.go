package casefile

import "strings"

func UserFacingLimitations(missing []MissingData) []string {
	if len(missing) == 0 {
		return nil
	}
	out := make([]string, 0, len(missing))
	for _, md := range missing {
		if md.Key == "" || md.Reason == "" {
			continue
		}
		if strings.HasPrefix(md.Key, "llm_") || strings.HasPrefix(md.Key, "tool:") {
			continue
		}
		out = append(out, md.Reason)
	}
	return out
}
