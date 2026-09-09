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
	serial   bool
	queue    [][]interface{}
	head     int
	size     int
	capacity int
	cond     *sync.Cond
	overflow OverflowPolicy
	onDrop   func(delta uint64)
	closed   bool
	done     chan struct{}
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
		for l.size == 0 && !l.closed {
			l.cond.Wait()
		}
		if l.size == 0 {
			l.mux.Unlock()
			return
		}
		args := l.queue[l.head]
		l.queue[l.head] = nil
		l.head = (l.head + 1) % l.capacity
		l.size--
		l.cond.Broadcast()
		l.mux.Unlock()

		l.call(args...)
	}
}

// deliver takes a shallow snapshot of args and enqueues it for the serial
// dispatch goroutine. Queue mutation is serialized by l.mux, but the drop
// callback always runs after the lock is released so it may safely publish,
// unsubscribe, or close the bus.
func (l *Listener) deliver(args []interface{}) {
	queuedArgs := append([]interface{}(nil), args...)

	l.mux.Lock()
	for l.overflow == OverflowBlock && l.size == l.capacity && !l.closed {
		l.cond.Wait()
	}
	if l.closed {
		l.mux.Unlock()
		return
	}

	dropped := false
	if l.size == l.capacity {
		l.queue[l.head] = nil
		l.head = (l.head + 1) % l.capacity
		l.size--
		dropped = true
	}
	tail := (l.head + l.size) % l.capacity
	l.queue[tail] = queuedArgs
	l.size++
	l.cond.Signal()
	onDrop := l.onDrop
	l.mux.Unlock()

	if dropped && onDrop != nil {
		onDrop(1)
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
