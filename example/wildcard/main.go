package main

import (
	"fmt"
	"github.com/hyponet/eventbus/bus"
)

var (
	waitBob   = make(chan struct{})
	waitAlice = make(chan struct{})
)

func bobDoSomething() {
	fmt.Println("Bob do something")
	close(waitBob)
}

func aliceDoSomething() {
	fmt.Println("Alice do something")
	close(waitAlice)
}

func main() {
	_, _ = bus.Subscribe("partner.bob.do", bobDoSomething)
	_, _ = bus.Subscribe("partner.alice.do", aliceDoSomething)
	bus.Publish("partner.*.do")
	<-waitBob
	<-waitAlice
}
