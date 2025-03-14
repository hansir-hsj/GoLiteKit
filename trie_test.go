package golitekit

import (
	"testing"
)

type TestController struct {
	BaseController

	value string
}

func TestAddGet(t *testing.T) {
	cases := []struct {
		path       string
		controller Controller
	}{
		{"/home", &TestController{}},
		{"/about", &TestController{}},
		{"/user/:id", &TestController{}},
		{"/user/:id/aaa", &TestController{value: "aaa"}},
		{"/user/:id/bbb", &TestController{value: "bbb"}},
	}

	trie := NewTrie()

	for _, c := range cases {
		trie.Add(c.path, c.controller)
	}

	for _, c := range cases {
		controller, _, _ := trie.Get(c.path)
		if controller == nil {
			t.Errorf("controller not found for path %s", c.path)
		}
		if controller != c.controller {
			t.Errorf("wrong controller found for path %s", c.path)
		}
	}
}

func TestWildPath(t *testing.T) {
	trie := NewTrie()
	trie.Add("/user/:id", &TestController{})
	if c, _, ok := trie.Get("/user/:id"); !ok || c == nil {
		t.Errorf("controller not found for path %s", "/user/:id")
	}
	if c, _, ok := trie.Get("/user/:name"); !ok || c == nil {
		t.Errorf("controller not found for path %s", "/user/:name")
	}
}

func TestDuplicatePath(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			if r != "duplicate path: /user/:name" {
				t.Errorf("Expected panic value 'duplicate path: /user/:name', but got %v", r)
			}
			t.Logf("Recovered: %v", r)
		}
	}()

	trie := NewTrie()
	trie.Add("/user/:id", &TestController{value: "id"})
	trie.Add("/user/:name", &TestController{value: "name"})
	t.Errorf("Expected panic, but test completed without panic")
}

func TestWildParams(t *testing.T) {
	trie := NewTrie()
	trie.Add("/user/:id/name", &TestController{})
	c, params, _ := trie.Get("/user/xiaoming/name")
	if c == nil {
		t.Errorf("controller not found for path %s", "/user/xiaoming/name")
	}
	if params["id"] != "xiaoming" {
		t.Errorf("wrong params found for path %s", "/user/xiaoming/name")
	}
}
