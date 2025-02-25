package env

import (
	"os"
	"path/filepath"
	"time"

	"github.com/hansir-hsj/GoLiteKit/config"
)

var defaultEnv = &Env{}

type EnvHttpServer struct {
	AppName string `toml:"appName"`
	RunMode string `toml:"runMode"`
	Addr    string `toml:"addr"`

	ReadTimeout       int `toml:"readTimeout"`
	ReadHeaderTimeout int `toml:"readHeaderTimeout"`
	WriteTimeout      int `toml:"writeTimeout"`
	IdleTimeout       int `toml:"idleTimeout"`
	ShutdownTimeout   int `toml:"shutdownTimeout"`

	MaxHeaderBytes int `toml:"maxHeaderBytes"`

	EnvRateLimit `toml:"RateLimit"`
	EnvLogger    `toml:"Logger"`
	EnvDB        `toml:"DB"`
	EnvTLSConfig `toml:"TLSConfig"`
}

type EnvRateLimit struct {
	RateLimit int `toml:"rateLimit"`
	RateBurst int `toml:"rateBurst"`
}

type EnvLogger struct {
	Logger string `toml:"configFile"`
}

type EnvDB struct {
	DB string `toml:"configFile"`
}

type EnvTLSConfig struct {
	CertFile string `toml:"certFile"`
	KeyFile  string `toml:"keyFile"`
}

type Env struct {
	RootDir string
	ConfDir string

	EnvHttpServer `toml:"HttpServer"`
}

func Init(path string) error {
	curPath, err := os.Getwd()
	if err != nil {
		return err
	}
	defaultEnv.RootDir = curPath
	defaultEnv.ConfDir = filepath.Join(curPath, "conf")
	err = config.Parse(path, defaultEnv)
	if err != nil {
		return err
	}

	return nil
}

func AppName() string {
	return defaultEnv.AppName
}

func RunMode() string {
	return defaultEnv.RunMode
}

func Addr() string {
	return defaultEnv.Addr
}

func RootDir() string {
	return defaultEnv.RootDir
}

func ConfDir() string {
	return defaultEnv.ConfDir
}

func ReadTimeout() time.Duration {
	if defaultEnv.ReadTimeout == 0 {
		return 200 * time.Millisecond
	}
	return time.Duration(defaultEnv.ReadTimeout) * time.Millisecond
}

func ReadHeaderTimeout() time.Duration {
	if defaultEnv.ReadHeaderTimeout == 0 {
		return 100 * time.Millisecond
	}
	return time.Duration(defaultEnv.ReadHeaderTimeout) * time.Millisecond
}

func WriteTimeout() time.Duration {
	if defaultEnv.WriteTimeout == 0 {
		return 500 * time.Millisecond
	}
	return time.Duration(defaultEnv.WriteTimeout) * time.Millisecond
}

func IdleTimeout() time.Duration {
	if defaultEnv.IdleTimeout == 0 {
		return 2 * time.Second
	}
	return time.Duration(defaultEnv.IdleTimeout) * time.Millisecond
}

func ShutdownTimeout() time.Duration {
	if defaultEnv.ShutdownTimeout == 0 {
		return 2 * time.Second
	}
	return time.Duration(defaultEnv.ShutdownTimeout) * time.Millisecond
}

func MaxHeaderBytes() int {
	if defaultEnv.MaxHeaderBytes == 0 {
		return 1 << 20
	}
	return defaultEnv.MaxHeaderBytes
}

func RateLimit() int {
	return defaultEnv.RateLimit
}

func RateBurst() int {
	if defaultEnv.RateBurst == 0 {
		return defaultEnv.RateLimit
	}
	return defaultEnv.RateBurst
}

func DBConfigFile() string {
	if defaultEnv.DB == "" {
		return ""
	}
	return filepath.Join(ConfDir(), defaultEnv.DB)
}

func LoggerConfigFile() string {
	if defaultEnv.Logger == "" {
		return ""
	}
	return filepath.Join(ConfDir(), defaultEnv.Logger)
}

func TLSCertFile() string {
	if defaultEnv.CertFile == "" {
		return ""
	}
	return filepath.Join(ConfDir(), defaultEnv.CertFile)
}

func TLSKeyFile() string {
	if defaultEnv.KeyFile == "" {
		return ""
	}
	return filepath.Join(ConfDir(), defaultEnv.KeyFile)
}
