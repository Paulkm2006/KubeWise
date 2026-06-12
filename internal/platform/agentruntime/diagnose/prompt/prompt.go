package prompt

import _ "embed"

// System prompts for diagnose LLM steps. Edit the .md files to tune behavior.

//go:embed supplemental_collect.system.md
var supplementalCollectSystem string

//go:embed log_evidence.system.md
var logEvidenceSystem string

//go:embed hypothesis_propose.system.md
var hypothesisProposeSystem string

//go:embed verify_judge.system.md
var verifyJudgeSystem string

//go:embed root_select.system.md
var rootSelectSystem string

func SupplementalCollectSystem() string { return supplementalCollectSystem }

func LogEvidenceSystem() string { return logEvidenceSystem }

func HypothesisProposeSystem() string { return hypothesisProposeSystem }

func VerifyJudgeSystem() string { return verifyJudgeSystem }

func RootSelectSystem() string { return rootSelectSystem }
