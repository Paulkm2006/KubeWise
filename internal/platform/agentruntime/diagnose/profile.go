package diagnose

type Mode string

const (
	ModeDashboardStrict Mode = "dashboard_strict"
	ModeConversation    Mode = "conversation"
)

type Profile struct {
	Mode                 Mode
	MaxSupplementalTools int
}

func DashboardStrictProfile() Profile {
	return Profile{Mode: ModeDashboardStrict, MaxSupplementalTools: 2}
}

func ConversationProfile() Profile {
	return Profile{Mode: ModeConversation, MaxSupplementalTools: 3}
}
