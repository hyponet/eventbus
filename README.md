# eventbus

Lightweight eventbus for golang

## Usage

### Static topic

```go
package main

import (
	"fmt"
	"github.com/hyponet/eventbus/bus"
)

func aAndBComputer(a, b int) {
	fmt.Printf("%d + %d = %d\n", a, b, a+b)
}

func main() {
	_, _ = bus.Register("op.int.and", aAndBComputer)
	bus.Publish("op.int.and", 1, 2)
}
```

### Wildcard topic

```go
package main

import (
	"fmt"
	"github.com/hyponet/eventbus/bus"
)

func bobDoSomething() {
	fmt.Println("Bob do something")
}

func aliceDoSomething() {
	fmt.Println("Alice do something")
}

func main() {
	_, _ = bus.Register("partner.bob.do", bobDoSomething)
	_, _ = bus.Register("partner.alice.do", aliceDoSomething)
	bus.Publish("partner.*.do")
}
```

## Benchmark

### Publish

```bash
goos: darwin
goarch: amd64
pkg: github.com/hyponet/eventbus
cpu: Intel(R) Core(TM) i7-8700B CPU @ 3.20GHz

BenchmarkBusPublish
BenchmarkBusPublish-12    	   10000	      2058 ns/op	     518 B/op	      15 allocs/op

BenchmarkBusPublish
BenchmarkBusPublish-12    	  100000	      1819 ns/op	     500 B/op	      15 allocs/op

BenchmarkBusPublish
BenchmarkBusPublish-12    	 1000000	      1918 ns/op	     498 B/op	      15 allocs/op

BenchmarkBusPublish
BenchmarkBusPublish-12    	10000000	      1695 ns/op	     497 B/op	      15 allocs/op
```

### Register
```bash
goos: darwin
goarch: amd64
pkg: github.com/hyponet/eventbus
cpu: Intel(R) Core(TM) i7-8700B CPU @ 3.20GHz

BenchmarkBusRegister
BenchmarkBusRegister-12    	   10000	      2604 ns/op	     962 B/op	      16 allocs/op

BenchmarkBusRegister
BenchmarkBusRegister-12    	  100000	      3451 ns/op	     951 B/op	      16 allocs/op

BenchmarkBusRegister
BenchmarkBusRegister-12    	 1000000	      3212 ns/op	    1074 B/op	      16 allocs/op

BenchmarkBusRegister
BenchmarkBusRegister-12    	10000000	      3303 ns/op	    1016 B/op	      16 allocs/op
```