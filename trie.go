package golitekit

import "strings"

const (
	WildKey string = ":WILD"
)

type Trie struct {
	root *Node
}

type Node struct {
	children     map[string]*Node
	controller   Controller
	hasWildChild bool
	word         string
}

func NewTrie() *Trie {
	return &Trie{
		root: newNode(),
	}
}

func newNode() *Node {
	return &Node{
		children: make(map[string]*Node),
	}
}

func isWildWord(word string) bool {
	return strings.HasPrefix(word, ":")
}

// These paths are identical, duplicate paths are not allowed
// /user/:id/name
// /user/:status/name
func (t *Trie) Add(path string, controller Controller) {
	trimed := strings.Trim(path, "/")
	words := strings.Split(trimed, "/")
	node := t.root

	for _, w := range words {
		if w == "" {
			continue
		}
		if isWildWord(w) {
			if node.hasWildChild {
				node = node.children[WildKey]
				if node.word != w[1:] {
					panic("duplicate path: " + path)
				}
				continue
			}
			child := newNode()
			child.word = w[1:]
			node.children[WildKey] = child
			node.hasWildChild = true
			node = child
		} else {
			if child, ok := node.children[w]; ok {
				node = child
				continue
			}
			child := newNode()
			node.children[w] = child
			node = child
		}
	}
	if node.controller != nil {
		panic("duplicate path: " + path)
	}
	node.controller = controller
}

// Add path /user/:id/name
// Get path /user/123456/name
// params: id = 123456
func (t *Trie) Get(path string) (Controller, map[string]string, bool) {
	trimed := strings.Trim(path, "/")
	words := strings.Split(trimed, "/")
	node := t.root
	params := make(map[string]string)

	for i, w := range words {
		if w == "" {
			continue
		}
		isLast := i == len(words)-1
		var child *Node
		child, ok := node.children[w]
		if !ok && node.hasWildChild {
			child = node.children[WildKey]
			params[child.word] = w
		}

		if child != nil {
			node = child
		}

		if isLast && node.controller != nil {
			return node.controller, params, true
		}
	}

	return nil, nil, false
}
