package event

// IsInteraction reports whether ev is a unified interaction request.
func IsInteraction(ev Event) (InteractionRequest, bool) {
	req, ok := ev.(InteractionRequest)
	return req, ok
}
