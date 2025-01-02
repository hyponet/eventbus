package main

import (
	"fmt"
	"github.com/hyponet/eventbus"
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
	eventbus.Subscribe("partner.bob.do", bobDoSomething)
	eventbus.Subscribe("partner.alice.do", aliceDoSomething)
	eventbus.Publish("partner.*.do")
	<-waitBob
	<-waitAlice
}
