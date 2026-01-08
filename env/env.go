package env

import (
	"os"
	"path/filepath"
	"time"

	"github.com/hansir-hsj/GoLiteKit/config"
)

const (
	DefaultReadTimeout       = 1000 * time.Millisecond
	DefaultWriteTimeout      = 1000 * time.Millisecond
	DefaultReadHeaderTimeout = 200 * time.Millisecond
	DefaultIdleTimeout       = 2 * time.Second
	DefaultShutdownTimeout   = 2 * time.Second
)

var (
	defaultEnv *Env
)

type EnvHttpServer struct {
	AppName string `toml:"appName"`
	RunMode string `toml:"runMode"`
	Network string `toml:"network"`
	Addr    string `toml:"addr"`

	MaxHeaderBytes int  `toml:"maxHeaderBytes"`
	EnablePprof    bool `toml:"enablePprof"`

	EnvTimeout   `toml:"Timeout"`
	EnvRateLimit `toml:"RateLimit"`
	EnvLogger    `toml:"Logger"`
	EnvDB        `toml:"DB"`
	EnvRedis     `toml:"Redis"`
	EnvTLSConfig `toml:"TLSConfig"`
	EnvSSE       `toml:"SSE"`
}

type EnvTimeout struct {
	ReadTimeout       int `toml:"readTimeout"`
	ReadHeaderTimeout int `toml:"readHeaderTimeout"`
	WriteTimeout      int `toml:"writeTimeout"`
	IdleTimeout       int `toml:"idleTimeout"`
	ShutdownTimeout   int `toml:"shutdownTimeout"`
}

type EnvRateLimit struct {
	RateLimit int `toml:"rateLimit"`
	RateBurst int `toml:"rateBurst"`
}

type EnvLogger struct {
	Logger          string `toml:"configFile"`
	LogRequestBody  bool   `toml:"logRequestBody"`
	LogResponseBody bool   `toml:"logResponseBody"`
}

type EnvDB struct {
	DB string `toml:"configFile"`
}

type EnvRedis struct {
	Redis string `toml:"configFile"`
}

type EnvTLSConfig struct {
	TLS      bool   `toml:"tls"`
	CertFile string `toml:"certFile"`
	KeyFile  string `toml:"keyFile"`
}

type EnvSSE struct {
	Timeout int `toml:"timeout"`
}

type Env struct {
	rootDir string
	confDir string

	EnvHttpServer `toml:"HttpServer"`
}

func Init(path string) error {
	curPath, err := os.Getwd()
	if err != nil {
		return err
	}
	defaultEnv = &Env{
		rootDir: curPath,
		confDir: filepath.Join(curPath, "conf"),
	}
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

func Network() string {
	return defaultEnv.Network
}

func Addr() string {
	return defaultEnv.Addr
}

func RootDir() string {
	return defaultEnv.rootDir
}

func ConfDir() string {
	return defaultEnv.confDir
}

func ReadTimeout() time.Duration {
	if defaultEnv.ReadTimeout == 0 {
		return DefaultReadTimeout
	}
	return time.Duration(defaultEnv.ReadTimeout) * time.Millisecond
}

func ReadHeaderTimeout() time.Duration {
	if defaultEnv.ReadHeaderTimeout == 0 {
		return DefaultReadHeaderTimeout
	}
	return time.Duration(defaultEnv.ReadHeaderTimeout) * time.Millisecond
}

func WriteTimeout() time.Duration {
	if defaultEnv.WriteTimeout == 0 {
		return DefaultWriteTimeout
	}
	return time.Duration(defaultEnv.WriteTimeout) * time.Millisecond
}

func IdleTimeout() time.Duration {
	if defaultEnv.IdleTimeout == 0 {
		return DefaultIdleTimeout
	}
	return time.Duration(defaultEnv.IdleTimeout) * time.Millisecond
}

func ShutdownTimeout() time.Duration {
	if defaultEnv.ShutdownTimeout == 0 {
		return DefaultShutdownTimeout
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

func RedisConfigFile() string {
	if defaultEnv.Redis == "" {
		return ""
	}
	return filepath.Join(ConfDir(), defaultEnv.Redis)
}

func LoggerConfigFile() string {
	if defaultEnv.Logger == "" {
		return ""
	}
	return filepath.Join(ConfDir(), defaultEnv.Logger)
}

func TLS() bool {
	return defaultEnv.TLS
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

func EnablePprof() bool {
	return defaultEnv.EnablePprof
}

func SSETimeout() time.Duration {
	if defaultEnv.Timeout == 0 {
		return 300 * time.Second
	}
	return time.Duration(defaultEnv.Timeout) * time.Millisecond
}

func LogRequestBody() bool {
	return defaultEnv.LogRequestBody
}

func LogResponseBody() bool {
	return defaultEnv.LogResponseBody
}
