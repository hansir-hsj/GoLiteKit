package env

import (
	"os"
	"path/filepath"
	"sync"
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
	envMu      sync.RWMutex
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
	EnvStatic    `toml:"Static"`
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

type EnvStatic struct {
	StaticDir string `toml:"staticDir"`
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
	nextEnv := &Env{
		rootDir: curPath,
		confDir: filepath.Join(curPath, "conf"),
	}
	if err := config.Parse(path, nextEnv); err != nil {
		return err
	}

	envMu.Lock()
	defaultEnv = nextEnv
	envMu.Unlock()
	return nil
}

func currentEnv() *Env {
	envMu.RLock()
	defer envMu.RUnlock()
	return defaultEnv
}

func AppName() string {
	e := currentEnv()
	if e == nil {
		return ""
	}
	return e.AppName
}

func RunMode() string {
	e := currentEnv()
	if e == nil {
		return ""
	}
	return e.RunMode
}

func Network() string {
	e := currentEnv()
	if e == nil {
		return ""
	}
	return e.Network
}

func Addr() string {
	e := currentEnv()
	if e == nil {
		return ""
	}
	return e.Addr
}

func RootDir() string {
	e := currentEnv()
	if e == nil {
		return ""
	}
	return e.rootDir
}

func ConfDir() string {
	e := currentEnv()
	if e == nil {
		return ""
	}
	return e.confDir
}

func StaticDir() string {
	e := currentEnv()
	if e == nil {
		return ""
	}
	if e.StaticDir == "" {
		return ""
	}
	return filepath.Join(e.rootDir, e.StaticDir)
}

func ReadTimeout() time.Duration {
	e := currentEnv()
	if e == nil {
		return DefaultReadTimeout
	}
	if e.ReadTimeout == 0 {
		return DefaultReadTimeout
	}
	return time.Duration(e.ReadTimeout) * time.Millisecond
}

func ReadHeaderTimeout() time.Duration {
	e := currentEnv()
	if e == nil {
		return DefaultReadHeaderTimeout
	}
	if e.ReadHeaderTimeout == 0 {
		return DefaultReadHeaderTimeout
	}
	return time.Duration(e.ReadHeaderTimeout) * time.Millisecond
}

func WriteTimeout() time.Duration {
	e := currentEnv()
	if e == nil {
		return DefaultWriteTimeout
	}
	if e.WriteTimeout == 0 {
		return DefaultWriteTimeout
	}
	return time.Duration(e.WriteTimeout) * time.Millisecond
}

func IdleTimeout() time.Duration {
	e := currentEnv()
	if e == nil {
		return DefaultIdleTimeout
	}
	if e.IdleTimeout == 0 {
		return DefaultIdleTimeout
	}
	return time.Duration(e.IdleTimeout) * time.Millisecond
}

func ShutdownTimeout() time.Duration {
	e := currentEnv()
	if e == nil {
		return DefaultShutdownTimeout
	}
	if e.ShutdownTimeout == 0 {
		return DefaultShutdownTimeout
	}
	return time.Duration(e.ShutdownTimeout) * time.Millisecond
}

func MaxHeaderBytes() int {
	e := currentEnv()
	if e == nil {
		return 1 << 20
	}
	if e.MaxHeaderBytes == 0 {
		return 1 << 20
	}
	return e.MaxHeaderBytes
}

func RateLimit() int {
	e := currentEnv()
	if e == nil {
		return 0
	}
	return e.RateLimit
}

func RateBurst() int {
	e := currentEnv()
	if e == nil {
		return 0
	}
	if e.RateBurst == 0 {
		return e.RateLimit
	}
	return e.RateBurst
}

func DBConfigFile() string {
	e := currentEnv()
	if e == nil {
		return ""
	}
	if e.DB == "" {
		return ""
	}
	return filepath.Join(e.confDir, e.DB)
}

func RedisConfigFile() string {
	e := currentEnv()
	if e == nil {
		return ""
	}
	if e.Redis == "" {
		return ""
	}
	return filepath.Join(e.confDir, e.Redis)
}

func LoggerConfigFile() string {
	e := currentEnv()
	if e == nil {
		return ""
	}
	if e.Logger == "" {
		return ""
	}
	return filepath.Join(e.confDir, e.Logger)
}

func TLS() bool {
	e := currentEnv()
	if e == nil {
		return false
	}
	return e.TLS
}

func TLSCertFile() string {
	e := currentEnv()
	if e == nil {
		return ""
	}
	if e.CertFile == "" {
		return ""
	}
	return filepath.Join(e.confDir, e.CertFile)
}

func TLSKeyFile() string {
	e := currentEnv()
	if e == nil {
		return ""
	}
	if e.KeyFile == "" {
		return ""
	}
	return filepath.Join(e.confDir, e.KeyFile)
}

func EnablePprof() bool {
	e := currentEnv()
	if e == nil {
		return false
	}
	return e.EnablePprof
}

func SSETimeout() time.Duration {
	e := currentEnv()
	if e == nil {
		return 300 * time.Second
	}
	if e.Timeout == 0 {
		return 300 * time.Second
	}
	return time.Duration(e.Timeout) * time.Millisecond
}

func LogRequestBody() bool {
	e := currentEnv()
	if e == nil {
		return false
	}
	return e.LogRequestBody
}

func LogResponseBody() bool {
	e := currentEnv()
	if e == nil {
		return false
	}
	return e.LogResponseBody
}
