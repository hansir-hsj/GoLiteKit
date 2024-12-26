package config

type Decoder func(data []byte) (any, error)

type Config interface {
	Parse(path string, obj any) error

	ParseBytes(data []byte, obj any) error

	Register(ext string, decoder Decoder)
}

type AppConfig struct {
}
