package main

import (
	"fmt"
	"github.com/hyponet/eventbus/bus"
)

var waiting = make(chan struct{})

func aAndBComputer(a, b int) {
	fmt.Printf("%d + %d = %d\n", a, b, a+b)
	close(waiting)
}

func main() {
	_, _ = bus.Subscribe("op.int.and", aAndBComputer)
	bus.Publish("op.int.and", 1, 2)
	<-waiting
}
