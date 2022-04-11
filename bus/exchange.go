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
		m := getSameSections(prefix, sections, false)
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
		m := getSameSections(prefix, sections, false)
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

type pos struct {
	idx  int
	next *radixNode
}

func lookupRadixTree(root *radixNode, topic string) (values []string) {
	var (
		queue    = []pos{{idx: 0, next: root.children}}
		sections = strings.Split(topic, sectionDelimiter)
	)
	for len(queue) > 0 {
		var (
			crt        = queue[0].next
			idx        = queue[0].idx
			isWildcard bool
		)
		queue = queue[1:]

		for crt != nil {
			prefix := strings.Split(crt.prefix, sectionDelimiter)
			subSections := sections[idx:]
			m := getSameSections(prefix, sections[idx:], true)
			if m == 0 {
				crt = crt.next
				continue
			}

			if m < len(prefix) {
				break
			}

			isWildcard = prefix[m-1] == oneSectionWildcard || subSections[m-1] == oneSectionWildcard

			if m < len(subSections) {
				queue = append(queue, pos{idx: idx + m, next: crt.children})
				if isWildcard {
					crt = crt.next
					continue
				}
				break
			}

			values = append(values, crt.values...)
			if isWildcard {
				crt = crt.next
				continue
			}
			break
		}
	}

	return
}

func getSameSections(prefix, pattern []string, withWildcard bool) int {
	m := 0
	for m < len(prefix) && m < len(pattern) {
		if withWildcard && (prefix[m] == oneSectionWildcard || pattern[m] == oneSectionWildcard) {
			m += 1
			continue
		}
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
