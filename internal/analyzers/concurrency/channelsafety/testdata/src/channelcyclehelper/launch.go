package channelcyclehelper

func Launch(first, second chan int) {
	go func() {
		first <- 1
		<-second
	}()
}

func Forward(first, second chan int) { Launch(first, second) }

func MaybeLaunch(first, second chan int, run bool) {
	if run {
		Launch(first, second)
	}
}

func LaunchInLoop(first, second chan int, count int) {
	for range count {
		Launch(first, second)
	}
}
