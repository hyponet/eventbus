package eventbus

import "github.com/hyponet/eventbus/bus"

type (
	Bus          = bus.Bus
	SerialOption = bus.SerialOption
)

var (
	Subscribe          = bus.Subscribe
	SubscribeSerial    = bus.SubscribeSerial
	SubscribeOnce      = bus.SubscribeOnce
	SubscribeWithBlock = bus.SubscribeWithBlock
	Unsubscribe        = bus.Unsubscribe
	Publish            = bus.Publish
	NewBus             = bus.NewBus
	WithBuffer         = bus.WithBuffer
	WithBlock          = bus.WithBlock
	WithDropCallback   = bus.WithDropCallback
	Close              = bus.Close
	Wait               = bus.Wait
)
