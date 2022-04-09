package bus

type exchange struct {
	root *radixNode
}

func (e *exchange) add(topic, lID string) {

}

func (e *exchange) remove(lID string) {

}

func (e *exchange) route(topic string) (listeners []string) {
	return nil
}

func newExchange() *exchange {
	return &exchange{root: &radixNode{}}
}

type radixNode struct {
	prefix    string
	listeners []string
	children  *radixNode
	peer      *radixNode
}

func insertToRadixTree(root *radixNode, topic, lID string) {

}

func removeFromRadixTree(root *radixNode, topic, lID string) {

}

func lookupRadixTree(root *radixNode, topic string) {

}
