package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hansir-hsj/GoLiteKit/config/test_data"
)

func TestParseEdgeCases(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.toml")
	yml := filepath.Join(dir, "data.YML")
	if err := os.WriteFile(empty, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(yml, []byte("name: bob\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var p test_data.Person
	if err := Parse(empty, &p); !errors.Is(err, ErrFileEmpty) {
		t.Errorf("empty file: err = %v, want ErrFileEmpty", err)
	}
	if err := Parse("", &p); !errors.Is(err, ErrPathEmpty) {
		t.Errorf("empty path: err = %v, want ErrPathEmpty", err)
	}
	if err := Parse(filepath.Join(dir, "missing.toml"), &p); err == nil || !strings.Contains(err.Error(), "file not found") {
		t.Errorf("missing file: err = %v", err)
	}
	if err := ParseBytes(ExtJSON, []byte(`{}`), nil); err == nil {
		t.Error("nil obj should return an error, not panic")
	}
	var nilPtr *test_data.Person
	if err := ParseBytes(ExtJSON, []byte(`{}`), nilPtr); err == nil {
		t.Error("nil pointer obj should return an error")
	}
	if err := Parse(yml, &p); err != nil {
		t.Errorf(".YML file: %v", err)
	}
}

// registerRun makes extension names unique across -count runs, since the
// decoder registry is process-wide.
var registerRun atomic.Int64

// Run with -race: Register used to write the decoder map without a lock.
func TestRegisterConcurrentWithParse(t *testing.T) {
	run := registerRun.Add(1)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			if err := Register(fmt.Sprintf(".concurrent%d_%d", run, i), JsonDecoder); err != nil {
				t.Errorf("Register: %v", err)
			}
		}(i)
		go func() {
			defer wg.Done()
			var p test_data.Person
			if err := ParseBytes(ExtJSON, []byte(`{}`), &p); err != nil {
				t.Errorf("ParseBytes: %v", err)
			}
		}()
	}
	wg.Wait()

	if err := Register(fmt.Sprintf(".CONCURRENT%d_0", run), JsonDecoder); err == nil {
		t.Fatal("duplicate Register (case-insensitive) should fail")
	}
}

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
