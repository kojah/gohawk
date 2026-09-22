package cancellationownership

import (
	"cancellationdep"
	"context"
)

func importedReturnedCancel() {
	_, cancel := context.WithCancel(context.Background())
	defer cancellationdep.CleanupFor(cancel)()
}

func importedReturnedWrongCancel() {
	_, cancel := context.WithCancel(context.Background()) // want "cancel function from context.WithCancel is not called"
	defer cancellationdep.CleanupFor(func() {})()
	_ = cancel
}
