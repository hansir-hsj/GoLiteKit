package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v2"
)

var (
	ErrFileEmpty  = errors.New("file is empty")
	ErrPathEmpty  = errors.New("config path is empty")
	defaultConfig = newAppConfig()
	// decodersMu guards defaultConfig.Decoders so Register may run
	// concurrently with Parse.
	decodersMu sync.RWMutex
)

const (
	ExtJSON = ".json"
	ExtTOML = ".toml"
	ExtYAML = ".yaml"
	ExtYML  = ".yml"
)

type Decoder func(data []byte, v any) error

type Config interface {
	Parse(path string, obj any) error

	ParseBytes(ext string, data []byte, obj any) error

	Register(ext string, decoder Decoder) error
}

type AppConfig struct {
	Decoders map[string]Decoder
}

func newAppConfig() *AppConfig {
	cnf := &AppConfig{
		Decoders: make(map[string]Decoder),
	}
	cnf.Decoders[ExtJSON] = JsonDecoder
	cnf.Decoders[ExtTOML] = TomlDecoder
	cnf.Decoders[ExtYAML] = YamlDecoder
	cnf.Decoders[ExtYML] = YamlDecoder

	return cnf
}

// Register adds a decoder for a file extension (e.g. ".ini"). Extensions are
// matched case-insensitively. It is safe to call concurrently with Parse.
func Register(ext string, decoder Decoder) error {
	if decoder == nil {
		return fmt.Errorf("decoder is nil")
	}
	ext = strings.ToLower(ext)
	decodersMu.Lock()
	defer decodersMu.Unlock()
	if _, ok := defaultConfig.Decoders[ext]; ok {
		return fmt.Errorf("decoder already registered for extension: %s", ext)
	}
	defaultConfig.Decoders[ext] = decoder

	return nil
}

// Parse reads the file at path and decodes it into obj, choosing the decoder
// by file extension.
func Parse(path string, obj any) error {
	data, err := ReadFile(path)
	if err != nil {
		return err
	}
	return ParseBytes(filepath.Ext(path), data, obj)
}

// ParseBytes decodes data into obj, which must be a non-nil pointer, using
// the decoder registered for ext. Empty data returns ErrFileEmpty.
func ParseBytes(ext string, data []byte, obj any) error {
	if len(data) == 0 {
		return ErrFileEmpty
	}

	decodersMu.RLock()
	decoder, ok := defaultConfig.Decoders[strings.ToLower(ext)]
	decodersMu.RUnlock()
	if !ok {
		return fmt.Errorf("decoder not found for extension: %s", ext)
	}

	v := reflect.ValueOf(obj)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return fmt.Errorf("obj must be a non-nil pointer, got %T", obj)
	}
	return decoder(data, obj)
}

// ReadFile reads the config file at path.
func ReadFile(path string) ([]byte, error) {
	if path == "" {
		return nil, ErrPathEmpty
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("file not found: %s", path)
	}
	return data, err
}

func JsonDecoder(data []byte, v any) error {
	err := json.Unmarshal(data, v)
	if err != nil {
		return err
	}
	return nil
}

func TomlDecoder(data []byte, v any) error {
	err := toml.Unmarshal(data, v)
	if err != nil {
		return err
	}
	return nil
}

func YamlDecoder(data []byte, v any) error {
	err := yaml.Unmarshal(data, v)
	if err != nil {
		return err
	}
	return nil
}
