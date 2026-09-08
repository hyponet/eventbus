package bus

import (
	"fmt"
	"math/rand"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

var gid uint64

func init() {
	gid = uint64(rand.Uint32())
}

const defaultSerialBuffer = 256

type Listener struct {
	id    string
	topic string
	fn    reflect.Value
	block bool
	once  bool
	mux   sync.Mutex

	// Serial delivery mode (see Bus.SubscribeSerial): a dedicated
	// goroutine drains queue in FIFO order so handler calls are strictly
	// ordered, unlike the default goroutine-per-publish dispatch.
	serial      bool
	queue       [][]interface{}
	capacity    int
	cond        *sync.Cond
	blockOnFull bool
	onDrop      func(topic string, dropped uint64)
	dropped     uint64
	closed      bool
	done        chan struct{}
}

func (l *Listener) call(args ...interface{}) {
	if l.block {
		l.mux.Lock()
		defer l.mux.Unlock()
	}
	l.fn.Call(l.parseArgs(args...))
}

func (l *Listener) parseArgs(inArgs ...interface{}) (args []reflect.Value) {
	fn := l.fn.Type()
	args = make([]reflect.Value, len(inArgs))
	for i, inArg := range inArgs {
		if inArg == nil {
			args[i] = reflect.New(fn.In(i)).Elem()
			continue
		}
		args[i] = reflect.ValueOf(inArg)
	}
	return args
}

// run is the serial dispatch loop: it delivers queued messages in FIFO order
// and exits after shutdown has been requested and the queue has been drained.
func (l *Listener) run() {
	for {
		l.mux.Lock()
		for len(l.queue) == 0 && !l.closed {
			l.cond.Wait()
		}
		if len(l.queue) == 0 {
			l.mux.Unlock()
			return
		}
		args := l.queue[0]
		l.queue[0] = nil
		l.queue = l.queue[1:]
		l.cond.Broadcast()
		l.mux.Unlock()

		l.call(args...)
	}
}

// deliver enqueues args for the serial dispatch goroutine. Queue mutation is
// serialized by l.mux, but the drop callback always runs after the lock is
// released so it may safely publish, unsubscribe, or close the bus.
func (l *Listener) deliver(args []interface{}) {
	l.mux.Lock()
	for l.blockOnFull && len(l.queue) >= l.capacity && !l.closed {
		l.cond.Wait()
	}
	if l.closed {
		l.mux.Unlock()
		return
	}

	var dropped uint64
	if len(l.queue) >= l.capacity {
		l.queue[0] = nil
		l.queue = l.queue[1:]
		l.dropped++
		dropped = l.dropped
	}
	l.queue = append(l.queue, args)
	l.cond.Signal()
	onDrop := l.onDrop
	topic := l.topic
	l.mux.Unlock()

	if dropped != 0 && onDrop != nil {
		onDrop(topic, dropped)
	}
}

// shutdown stops new deliveries exactly once. Buffered messages are still
// delivered before the dispatch goroutine exits.
func (l *Listener) shutdown() {
	l.mux.Lock()
	if !l.closed {
		l.closed = true
		l.cond.Broadcast()
	}
	l.mux.Unlock()
}

func NewListener(topic string, fn interface{}, block, once bool) *Listener {
	if fn == nil {
		panic("handler must be function")
	}

	handler := reflect.ValueOf(fn)
	if handler.Type().Kind() != reflect.Func {
		panic("handler not a function")
	}

	return &Listener{
		id:    fmt.Sprintf("%d.%d", time.Now().Nanosecond(), atomic.AddUint64(&gid, 1)),
		topic: topic,
		fn:    handler,
		block: block,
		once:  once,
	}
}
