package eventbus

import "sync"

var (
	eb  *evnetbus
	mux sync.RWMutex
)

func init() {
	eb = &evnetbus{
		stone:    map[string]listener{},
		registry: map[Topic][]listener{},
	}
}

type evnetbus struct {
	stone    map[string]listener
	registry map[Topic][]listener
}

func Register(t Topic, l listener) {
	mux.Lock()
	defer mux.Unlock()
	l.Topic = t

	eb.stone[l.ID] = l
	eb.registry[t] = append(eb.registry[t], l)
}

// Unregister TODO unregister support
func Unregister(ID string) {
	mux.Lock()
	defer mux.Unlock()
}

func Publish(t Topic, args ...interface{}) {
	mux.RLock()
	defer mux.RUnlock()

	listeners := eb.registry[t]
	for _, l := range listeners {
		l.do(args...)
	}
}
