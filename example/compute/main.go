package main

import (
	"fmt"
	"github.com/hyponet/eventbus"
)

var waiting = make(chan struct{})

func aAndBComputer(a, b int) {
	fmt.Printf("%d + %d = %d\n", a, b, a+b)
	close(waiting)
}

func main() {
	eventbus.Subscribe("op.int.and", aAndBComputer)
	eventbus.Publish("op.int.and", 1, 2)
	<-waiting
}
