package cancellationownership

import (
	"cancellationdep"
	"context"
)

func importedConditionalCancel(ready bool) {
	_, cancel := context.WithCancel(context.Background())
	if cancellationdep.CancelWhenReady(cancel, ready) {
		return
	}
	cancel()
}

func importedConditionalWrongCancel(ready bool) {
	_, cancel := context.WithCancel(context.Background()) // want "cancel function from context.WithCancel is not called"
	if cancellationdep.CancelWhenReady(func() {}, ready) {
		return
	}
	cancel()
}
