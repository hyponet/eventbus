package bus

import "strings"

const (
	sectionDelimiter   = "."
	oneSectionWildcard = "*"
)

type exchange struct {
	root         *radixNode
	topicMapping map[string]string
}

func (e *exchange) add(topic, lID string) {
	e.topicMapping[lID] = topic
	insertToRadixTree(e.root, topic, lID)
}

func (e *exchange) remove(lID string) {
	topic, ok := e.topicMapping[lID]
	if !ok {
		return
	}
	removeFromRadixTree(e.root, topic, lID)
	delete(e.topicMapping, lID)
}

func (e *exchange) route(topic string) (listeners []string) {
	listeners = lookupRadixTree(e.root, topic)
	return
}

func newExchange() *exchange {
	return &exchange{root: &radixNode{}, topicMapping: map[string]string{}}
}

type radixNode struct {
	prefix   string
	values   []string
	children *radixNode
	next     *radixNode
}

func insertToRadixTree(root *radixNode, topic, value string) {
	var (
		crt      = root.children
		sections = strings.Split(topic, sectionDelimiter)
	)

	if crt == nil {
		root.children = &radixNode{
			prefix: topic,
			values: []string{value},
		}
		return
	}

	for crt != nil {
		prefix := strings.Split(crt.prefix, sectionDelimiter)
		m := getSameSections(prefix, sections)
		if m == 0 {
			if crt.next == nil {
				crt.next = &radixNode{
					prefix: topic,
					values: []string{value},
				}
				return
			}
			crt = crt.next
			continue
		}

		if m == len(prefix) {
			if m == len(sections) {
				crt.values = append(crt.values, value)
				return
			}
			insertToRadixTree(crt, strings.Join(sections[m:], sectionDelimiter), value)
			return
		}

		split := &radixNode{
			prefix:   strings.Join(prefix[m:], sectionDelimiter),
			values:   crt.values,
			children: crt.children,
		}
		crt.prefix = strings.Join(prefix[:m], sectionDelimiter)
		crt.values = nil
		crt.children = split
		insertToRadixTree(crt, strings.Join(sections[m:], sectionDelimiter), value)
		return
	}
}

func removeFromRadixTree(root *radixNode, topic, value string) {
	var (
		crt      = root.children
		sections = strings.Split(topic, sectionDelimiter)
	)

	if crt == nil {
		return
	}

	for crt != nil {
		prefix := strings.Split(crt.prefix, sectionDelimiter)
		m := getSameSections(prefix, sections)
		if m == 0 {
			if crt.next == nil {
				return
			}
			crt = crt.next
			continue
		}

		if m < len(prefix) {
			return
		}

		if m < len(sections) {
			removeFromRadixTree(crt, strings.Join(sections[m:], sectionDelimiter), value)
			if crt.children != nil && crt.children.next == nil && len(crt.values) == 0 {
				// merge
				crt.prefix += "." + crt.children.prefix
				crt.values = crt.children.values
				crt.children = crt.children.children
			}
			return
		}

		// do delete
		crt.values = removeValues(crt.values, value)

		child := root.children
		lastChild := child
		for child != nil {
			if len(child.values) == 0 && child.children == nil {
				if child == lastChild {
					root.children = child.next
					break
				}
				lastChild.next = child.next
				break
			}
			lastChild = child
			child = child.next
		}

		if crt.children != nil && crt.children.next == nil && len(crt.values) == 0 {
			// merge
			crt.prefix += "." + crt.children.prefix
			crt.values = crt.children.values
			crt.children = crt.children.children
		}
		return
	}
}

func lookupRadixTree(root *radixNode, topic string) (values []string) {
	return nil
}

func getSameSections(prefix, pattern []string) int {
	m := 0
	for m < len(prefix) && m < len(pattern) {
		if prefix[m] != pattern[m] {
			break
		}
		m += 1
	}
	return m
}

func removeValues(values []string, value string) []string {
	for i, val := range values {
		if val != value {
			continue
		}
		return append(values[:i], values[i+1:]...)
	}
	return values
}
