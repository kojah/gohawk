package cancellationdep

func CancelWhenReady(cancel func(), ready bool) bool {
	if !ready {
		return false
	}
	cancel()
	return true
}
