package eventbus

import "github.com/hyponet/eventbus/bus"

type (
	Bus            = bus.Bus
	OverflowPolicy = bus.OverflowPolicy
	SerialConfig   = bus.SerialConfig
)

const (
	OverflowBlock      = bus.OverflowBlock
	OverflowDropOldest = bus.OverflowDropOldest
)

var (
	Subscribe          = bus.Subscribe
	SubscribeSerial    = bus.SubscribeSerial
	SubscribeOnce      = bus.SubscribeOnce
	SubscribeWithBlock = bus.SubscribeWithBlock
	Unsubscribe        = bus.Unsubscribe
	Publish            = bus.Publish
	NewBus             = bus.NewBus
	Close              = bus.Close
	Wait               = bus.Wait
)
