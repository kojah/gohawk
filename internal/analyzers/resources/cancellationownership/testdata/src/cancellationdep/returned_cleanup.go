package cancellationdep

func CleanupFor(cancel func()) func() {
	return func() { cancel() }
}
