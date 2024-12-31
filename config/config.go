package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v2"
)

var (
	ErrFileEmpty  = errors.New("file is empty")
	defaultConfig = newAppConfig()
)

const (
	ExtJSON = ".json"
	ExtTOML = ".toml"
	ExtYAML = ".yaml"
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

	return cnf
}

func Register(ext string, decoder Decoder) error {
	if decoder == nil {
		return fmt.Errorf("decoder is nil")
	}
	if _, ok := defaultConfig.Decoders[ext]; ok {
		return fmt.Errorf("decoder already registered for extension: %s", ext)
	}
	defaultConfig.Decoders[ext] = decoder

	return nil
}

func Parse(path string, obj any) error {
	data, err := ReadFile(path)
	if err != nil {
		return err
	}
	ext := filepath.Ext(path)

	return ParseBytes(ext, data, obj)
}

func ParseBytes(ext string, data []byte, obj any) error {
	if data == nil {
		return ErrFileEmpty
	}

	decoder, ok := defaultConfig.Decoders[ext]
	if !ok {
		return fmt.Errorf("decoder not found for extension: %s", ext)
	}

	t := reflect.TypeOf(obj)
	if t.Kind() != reflect.Ptr {
		return fmt.Errorf("obj must be a pointer: %s", t)
	}
	err := decoder(data, obj)
	if err != nil {
		return err
	}

	return nil
}

func ReadFile(path string) ([]byte, error) {
	if path == "" {
		return nil, nil
	}
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %s", path)
		}
		return nil, err
	}
	return os.ReadFile(path)
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
