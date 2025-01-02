package eventbus

import (
	"github.com/hyponet/eventbus/bus"
	"testing"
)

var (
	topics = []string{
		"a.b.c.d.e.f.g",
		"a.b.c.d.e.f.h",
		"a.b.c.d.e.i.h",
		"a.b.c.d.g.i.h",
		"a.b.c.k.g.i.h",
		"a.d.c.k.g.i.h",
		"a.d.e.k.g.i.h",
		"a.d.l.k.g.i.h",
		"a.d.m.k.g.i.h",
	}

	wildcards = []string{
		"a.b.c.d.e.f.*",
		"a.b.c.d.e.*.*",
		"a.b.c.d.e.*.*",
		"a.d.*.k.g.i.h",
	}
)

func doNothing() {}

func init() {
	for _, topic := range topics {
		bus.Subscribe(topic, doNothing)
	}
}

func BenchmarkBusPublish(b *testing.B) {
	target := append(topics, wildcards...)
	for n := 0; n < b.N; n++ {
		bus.Publish(target[n%len(target)])
	}
}

func BenchmarkBusSubscribe(b *testing.B) {
	target := append(topics, wildcards...)
	for n := 0; n < b.N; n++ {
		bus.Subscribe(target[n%len(target)], doNothing)
	}
}
