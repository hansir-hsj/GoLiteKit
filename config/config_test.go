package config

import (
	"testing"

	"github.com/hansir-hsj/GoLiteKit/config/test_data"
)

func TestConfigParsing(t *testing.T) {
	t.Run("JSON", func(t *testing.T) {
		var p test_data.Person
		err := Parse("test_data/data.json", &p)
		if err != nil {
			t.Error(err)
		}
		t.Log(p)
	})

	t.Run("YAML", func(t *testing.T) {
		var p test_data.Person
		err := Parse("test_data/data.yaml", &p)
		if err != nil {
			t.Error(err)
		}
		t.Log(p)
	})

	t.Run("TOML", func(t *testing.T) {
		var p test_data.Person
		err := Parse("test_data/data.toml", &p)
		if err != nil {
			t.Error(err)
		}
		t.Log(p)
	})
}
