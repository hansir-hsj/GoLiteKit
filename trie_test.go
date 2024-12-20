package golite

import (
	"context"
	"testing"
)

type TestController struct {
	value string
}

func (tc TestController) Serve(ctx context.Context) error {
	return nil
}

func TestAddGet(t *testing.T) {
	cases := []struct {
		path       string
		controller Controller
	}{
		{"/home", TestController{}},
		{"/about", TestController{}},
		{"/user/:id", TestController{}},
		{"/user/:id/aaa", TestController{"aaa"}},
		{"/user/:id/bbb", TestController{"bbb"}},
		{"/user/:age/name", TestController{"name"}},
	}

	trie := NewTrie()

	for _, c := range cases {
		trie.Add(c.path, c.controller)
	}

	for _, c := range cases {
		controller, _ := trie.Get(c.path)
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
	trie.Add("/user/:id", TestController{})
	if c, ok := trie.Get("/user/:id"); !ok || c == nil {
		t.Errorf("controller not found for path %s", "/user/:id")
	}
	if c, ok := trie.Get("/user/:name"); !ok || c == nil {
		t.Errorf("controller not found for path %s", "/user/:id")
	}
}
