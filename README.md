# eventbus

EventBus is a very useful tool for event-driven style code. You can use events to chain different logic together without coupling components

## Usage

### Static topic

```go
package main

import (
	"fmt"
	"github.com/hyponet/eventbus"
)

func aAndBComputer(a, b int) {
	fmt.Printf("%d + %d = %d\n", a, b, a+b)
}

func main() {
	eventbus.Subscribe("op.int.and", aAndBComputer)
	eventbus.Publish("op.int.and", 1, 2)
}
```

### Wildcard topic

```go
package main

import (
	"fmt"
	"github.com/hyponet/eventbus"
)

func bobDoSomething() {
	fmt.Println("Bob do something")
}

func aliceDoSomething() {
	fmt.Println("Alice do something")
}

func main() {
	eventbus.Subscribe("partner.bob.do", bobDoSomething)
	eventbus.Subscribe("partner.alice.do", aliceDoSomething)
	eventbus.Publish("partner.*.do")
}
```

### Serial delivery

Use an isolated bus when one listener must process events from one or more
topics in enqueue order. Every serial listener owns one bounded FIFO queue and
one consumer goroutine, so its handler calls never overlap. The overflow policy
is required: choose `OverflowBlock` for lossless delivery with backpressure, or
`OverflowDropOldest` when keeping the newest queued events is more important.
`SerialConfig.OnDrop`, when set with `OverflowDropOldest`, receives a delta for
each notification so concurrent callbacks can safely contribute to one counter.

```go
package main

import "github.com/hyponet/eventbus"

func main() {
	b := eventbus.NewBus()
	lid := b.SubscribeSerial([]string{"audit.created", "audit.updated"}, func(id int) {
		// Calls across both topics never overlap and preserve FIFO order.
	}, eventbus.SerialConfig{
		Buffer:   1024,
		Overflow: eventbus.OverflowBlock,
	})

	b.Publish("audit.created", 1)
	b.Publish("audit.created", 2)
	b.Unsubscribe(lid)
	b.Wait() // wait for queued events to finish
}
```

The handler receives exactly the arguments passed to `Publish`; the matching
topic is not injected. A single publisher's call order is preserved across all
subscribed topics. Concurrent publishers are linearized by queue admission,
and no wall-clock ordering between concurrent calls is implied.

`Unsubscribe` and `Close` are safe to call from inside a serial handler. They
start an asynchronous drain; call `Wait` outside the handler when completion
must be observed. With `OverflowBlock`, a handler must not synchronously publish
to its own full queue because it is the only consumer that can release capacity.

## Benchmark

Run the full benchmark suite with:

```bash
go test . -run '^$' -bench '.' -benchmem -benchtime=1s -count=1
```

Result from an Intel Core i7-8700B running Darwin:

```bash
goos: darwin
goarch: amd64
pkg: github.com/hyponet/eventbus
cpu: Intel(R) Core(TM) i7-8700B CPU @ 3.20GHz
BenchmarkBusSubscribe-12                                             646203  2127 ns/op  1097 B/op  18 allocs/op
BenchmarkBusPublishEndToEnd/async/producers=1/consumers=1-12        1593606   781.5 ns/op  1279666 deliveries/s   176 B/op   6 allocs/op
BenchmarkBusPublishEndToEnd/async/producers=4/consumers=1-12        1409439   836.2 ns/op  1195923 deliveries/s   176 B/op   6 allocs/op
BenchmarkBusPublishEndToEnd/async/producers=16/consumers=1-12       1408899   846.9 ns/op  1180784 deliveries/s   176 B/op   6 allocs/op
BenchmarkBusPublishEndToEnd/async/producers=4/consumers=4-12         615376  2023 ns/op     1977191 deliveries/s   440 B/op  13 allocs/op
BenchmarkBusPublishEndToEnd/async/producers=4/consumers=16-12        208120  5965 ns/op     2682537 deliveries/s  1689 B/op  39 allocs/op
BenchmarkBusPublishEndToEnd/serial-block/producers=1/consumers=1-12 2239939   506.7 ns/op  1973410 deliveries/s   144 B/op   6 allocs/op
BenchmarkBusPublishEndToEnd/serial-block/producers=4/consumers=1-12 2122562   571.3 ns/op  1750395 deliveries/s   144 B/op   6 allocs/op
BenchmarkBusPublishEndToEnd/serial-block/producers=16/consumers=1-12 2032604   577.4 ns/op  1731880 deliveries/s   144 B/op   6 allocs/op
BenchmarkBusPublishEndToEnd/serial-block/producers=4/consumers=4-12  921002  1356 ns/op     2949926 deliveries/s   312 B/op  12 allocs/op
BenchmarkBusPublishEndToEnd/serial-block/producers=4/consumers=16-12 276670  4191 ns/op     3817292 deliveries/s  1176 B/op  38 allocs/op
BenchmarkBusPublishSerialTopics/topics=1/producers=4-12             1993556   610.6 ns/op  1637677 deliveries/s   208 B/op   6 allocs/op
BenchmarkBusPublishSerialTopics/topics=4/producers=4-12             1521656   778.8 ns/op  1283961 deliveries/s   256 B/op  10 allocs/op
BenchmarkBusPublishSerialTopics/topics=16/producers=4-12             880863  1341 ns/op      745510 deliveries/s   448 B/op  22 allocs/op
BenchmarkBusPublishSerialDropOldestSaturated/producers=1-12         3554383   327.0 ns/op  100.0 drop_pct   120 B/op  5 allocs/op
BenchmarkBusPublishSerialDropOldestSaturated/producers=4-12         2923212   400.5 ns/op  100.0 drop_pct   120 B/op  5 allocs/op
BenchmarkBusPublishSerialDropOldestSaturated/producers=16-12        2912940   404.7 ns/op  100.0 drop_pct   120 B/op  5 allocs/op
PASS
ok  github.com/hyponet/eventbus  92.977s
```

`BenchmarkBusPublishEndToEnd` includes publisher completion, every expected
handler call, and bus shutdown. `consumers=N` means N independently registered
listeners, so one publish produces N deliveries; `deliveries/s` reports total
handler throughput. The saturated DropOldest benchmark deliberately stalls its
consumer and measures admission/drop throughput rather than end-to-end work.
