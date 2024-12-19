package golite

import (
	"context"
	"testing"
)

type TestController struct{}

func (tc TestController) Serve(ctx context.Context) error {
	return nil
}

func TestTrie(t *testing.T) {
	cases := []struct {
		path       string
		controller Controller
	}{
		{"/home", TestController{}},
		{"/about", TestController{}},
		{"/user/:id", TestController{}},
		{"/user/:age/name", TestController{}},
	}

	trie := NewTrie()

	for _, c := range cases {
		trie.Add(c.path, c.controller)
	}

	for _, c := range cases {
		controller := trie.Get(c.path)
		if controller == nil {
			t.Errorf("controller not found for path %s", c.path)
		}
	}
}
