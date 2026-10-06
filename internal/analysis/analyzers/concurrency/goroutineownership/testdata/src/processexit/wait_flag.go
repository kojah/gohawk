package main

// waitForWorker makes a done channel a real join obligation: a closed
// channel that nothing ever receives from is not one. The path that skips
// the wait is the one each fixture judges.
var waitForWorker bool
