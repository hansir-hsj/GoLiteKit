package config

import (
	"github/hsj/golite/config/test_data"
	"testing"
)

func TestConfigParsing(t *testing.T) {
	t.Run("JSON", func(t *testing.T) {
		cnf := NewAppConfig()
		var p test_data.Person
		err := cnf.Parse("test_data/data.json", &p)
		if err != nil {
			t.Error(err)
		}
		t.Log(p)
	})

	t.Run("YAML", func(t *testing.T) {
		cnf := NewAppConfig()
		var p test_data.Person
		err := cnf.Parse("test_data/data.yaml", &p)
		if err != nil {
			t.Error(err)
		}
		t.Log(p)
	})

	t.Run("TOML", func(t *testing.T) {
		cnf := NewAppConfig()
		var p test_data.Person
		err := cnf.Parse("test_data/data.toml", &p)
		if err != nil {
			t.Error(err)
		}
		t.Log(p)
	})
}
