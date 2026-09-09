package eventbus

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hyponet/eventbus/bus"
)

var benchmarkTopics = []string{
	"a.b.c.d.e.f.g",
	"a.b.c.d.e.f.h",
	"a.b.c.d.e.i.h",
	"a.b.c.d.g.i.h",
	"a.b.c.k.g.i.h",
	"a.d.c.k.g.i.h",
	"a.d.e.k.g.i.h",
	"a.d.l.k.g.i.h",
	"a.d.m.k.g.i.h",
	"a.b.c.d.e.f.*",
	"a.b.c.d.e.*.*",
	"a.d.*.k.g.i.h",
}

func BenchmarkBusSubscribe(b *testing.B) {
	eb := bus.NewBus()
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		topic := benchmarkTopics[n%len(benchmarkTopics)]
		eb.Subscribe(bus.NewListener(topic, func() {}, false, false))
	}
	b.StopTimer()
	eb.Close()
}

// BenchmarkBusPublishEndToEnd measures a complete lossless publish cycle:
// all producers finish, every matching consumer handles every message, and
// the bus shuts down. Consumers means independently registered listeners;
// each published message therefore produces consumers handler calls.
func BenchmarkBusPublishEndToEnd(b *testing.B) {
	type workload struct {
		producers int
		consumers int
	}
	workloads := []workload{
		{producers: 1, consumers: 1},
		{producers: 4, consumers: 1},
		{producers: 16, consumers: 1},
		{producers: 4, consumers: 4},
		{producers: 4, consumers: 16},
	}

	for _, serial := range []bool{false, true} {
		mode := "async"
		if serial {
			mode = "serial-block"
		}
		for _, workload := range workloads {
			workload := workload
			b.Run(fmt.Sprintf("%s/producers=%d/consumers=%d", mode, workload.producers, workload.consumers), func(b *testing.B) {
				benchmarkEndToEnd(b, serial, workload.producers, workload.consumers)
			})
		}
	}
}

func benchmarkEndToEnd(b *testing.B, serial bool, producers, consumers int) {
	const topic = "benchmark.end_to_end"
	eb := bus.NewBus()
	expected := int64(b.N) * int64(consumers)
	var handled atomic.Int64
	done := make(chan struct{})
	handler := func(int) {
		if handled.Add(1) == expected {
			close(done)
		}
	}

	for i := 0; i < consumers; i++ {
		if serial {
			eb.SubscribeSerial([]string{topic}, handler, bus.SerialConfig{
				Buffer:   1024,
				Overflow: bus.OverflowBlock,
			})
			continue
		}
		eb.Subscribe(bus.NewListener(topic, handler, false, false))
	}

	runPublishers := preparePublishers(b.N, producers, func(n int) {
		eb.Publish(topic, n)
	})
	b.ReportAllocs()
	b.ResetTimer()
	runPublishers()
	eb.Close()
	if serial {
		eb.Wait()
	}
	<-done
	b.StopTimer()

	if got := handled.Load(); got != expected {
		b.Fatalf("handled %d messages, want %d", got, expected)
	}
	b.ReportMetric(float64(handled.Load())/b.Elapsed().Seconds(), "deliveries/s")
}

// BenchmarkBusPublishSerialTopics measures the routing and queueing cost of
// merging more topics into one ordered listener while four producers compete.
func BenchmarkBusPublishSerialTopics(b *testing.B) {
	for _, topicCount := range []int{1, 4, 16} {
		b.Run(fmt.Sprintf("topics=%d/producers=4", topicCount), func(b *testing.B) {
			topics := make([]string, topicCount)
			for i := range topics {
				topics[i] = fmt.Sprintf("benchmark.serial.topic.%d", i)
			}

			eb := bus.NewBus()
			var handled atomic.Int64
			eb.SubscribeSerial(topics, func(int) {
				handled.Add(1)
			}, bus.SerialConfig{Buffer: 1024, Overflow: bus.OverflowBlock})
			runPublishers := preparePublishers(b.N, 4, func(n int) {
				eb.Publish(topics[n%len(topics)], n)
			})

			b.ReportAllocs()
			b.ResetTimer()
			runPublishers()
			eb.Close()
			eb.Wait()
			b.StopTimer()

			if got := handled.Load(); got != int64(b.N) {
				b.Fatalf("handled %d messages, want %d", got, b.N)
			}
			b.ReportMetric(float64(handled.Load())/b.Elapsed().Seconds(), "deliveries/s")
		})
	}
}

// BenchmarkBusPublishSerialDropOldestSaturated measures admission throughput
// while the only consumer is stalled and the bounded queue stays full. Unlike
// the end-to-end benchmark, handler drain time is deliberately excluded.
func BenchmarkBusPublishSerialDropOldestSaturated(b *testing.B) {
	for _, producers := range []int{1, 4, 16} {
		b.Run(fmt.Sprintf("producers=%d", producers), func(b *testing.B) {
			const topic = "benchmark.drop_oldest"
			eb := bus.NewBus()
			handlerStarted := make(chan struct{})
			releaseHandler := make(chan struct{})
			var (
				startedOnce sync.Once
				dropped     atomic.Uint64
			)
			eb.SubscribeSerial([]string{topic}, func(int) {
				startedOnce.Do(func() { close(handlerStarted) })
				<-releaseHandler
			}, bus.SerialConfig{
				Buffer:   64,
				Overflow: bus.OverflowDropOldest,
				OnDrop:   func(delta uint64) { dropped.Add(delta) },
			})
			eb.Publish(topic, -1)
			<-handlerStarted

			runPublishers := preparePublishers(b.N, producers, func(n int) {
				eb.Publish(topic, n)
			})
			b.ReportAllocs()
			b.ResetTimer()
			runPublishers()
			b.StopTimer()

			dropCount := dropped.Load()
			if b.N > 64 && dropCount == 0 {
				b.Fatal("saturated benchmark did not drop messages")
			}
			b.ReportMetric(100*float64(dropCount)/float64(b.N), "drop_pct")
			close(releaseHandler)
			eb.Close()
			eb.Wait()
		})
	}
}

// preparePublishers creates producers before timing starts and evenly
// partitions total publishes between them without adding a shared atomic
// counter to the measured path.
func preparePublishers(total, producers int, publish func(int)) func() {
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(producers)
	for producer := 0; producer < producers; producer++ {
		go func(producer int) {
			defer wg.Done()
			<-start
			for n := producer; n < total; n += producers {
				publish(n)
			}
		}(producer)
	}
	return func() {
		close(start)
		wg.Wait()
	}
}
