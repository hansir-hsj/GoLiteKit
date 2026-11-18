package env

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hansir-hsj/GoLiteKit/config"
)

// Defaults match golitekit.DefaultServerConfig. WriteTimeout also bounds the
// handler duration in NewAppFromConfig, so it must not be sub-second.
const (
	DefaultReadTimeout       = 30 * time.Second
	DefaultWriteTimeout      = 30 * time.Second
	DefaultReadHeaderTimeout = 5 * time.Second
	DefaultIdleTimeout       = 60 * time.Second
	DefaultShutdownTimeout   = 10 * time.Second
	DefaultMaxHeaderBytes    = 1 << 20
	DefaultSSETimeout        = 300 * time.Second
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
	// PprofAddr, if set with enablePprof, serves pprof on its own listener
	// (e.g. "127.0.0.1:6060") instead of the app's port.
	PprofAddr string `toml:"pprofAddr"`

	EnvTimeout   `toml:"Timeout"`
	EnvLogger    `toml:"Logger"`
	EnvDB        `toml:"DB"`
	EnvRedis     `toml:"Redis"`
	EnvTLSConfig `toml:"TLSConfig"`
	EnvSSE       `toml:"SSE"`
	EnvStatic    `toml:"Static"`
}

// EnvTimeout values are in milliseconds.
type EnvTimeout struct {
	ReadTimeout       int `toml:"readTimeout"`
	ReadHeaderTimeout int `toml:"readHeaderTimeout"`
	WriteTimeout      int `toml:"writeTimeout"`
	IdleTimeout       int `toml:"idleTimeout"`
	ShutdownTimeout   int `toml:"shutdownTimeout"`
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

// EnvSSE.Timeout is in milliseconds.
type EnvSSE struct {
	Timeout int `toml:"timeout"`
}

// Env is one loaded app config; its methods are nil-safe and return defaults.
// Referenced files (configFile, certFile, keyFile) are relative to ConfDir,
// staticDir to RootDir; absolute paths are kept.
type Env struct {
	rootDir string
	confDir string

	HttpServer EnvHttpServer `toml:"HttpServer"`
}

// Load reads the app config at path into a new Env without changing the
// process-wide default. Use it to run several apps with different configs in
// one process.
func Load(path string) (*Env, error) {
	curPath, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	e := &Env{
		rootDir: curPath,
		confDir: filepath.Dir(absPath),
	}
	if err := config.Parse(path, e); err != nil {
		return nil, err
	}
	return e, nil
}

// Init loads the app config at path and makes it the process-wide default
// read by the package-level functions (AppName, Addr, ...).
func Init(path string) error {
	e, err := Load(path)
	if err != nil {
		return err
	}
	SetDefault(e)
	return nil
}

// SetDefault makes e the Env read by the package-level functions.
func SetDefault(e *Env) {
	envMu.Lock()
	defaultEnv = e
	envMu.Unlock()
}

// Default returns the Env set by Init or SetDefault, or nil.
func Default() *Env {
	envMu.RLock()
	defer envMu.RUnlock()
	return defaultEnv
}

func millis(v int, def time.Duration) time.Duration {
	if v == 0 {
		return def
	}
	return time.Duration(v) * time.Millisecond
}

// resolvePath returns p unchanged if it is empty or absolute, otherwise p
// joined to base.
func resolvePath(base, p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(base, p)
}

// ---- accessors on *Env ----------------------------------------------------

func (e *Env) AppName() string {
	if e == nil {
		return ""
	}
	return e.HttpServer.AppName
}

func (e *Env) RunMode() string {
	if e == nil {
		return ""
	}
	return e.HttpServer.RunMode
}

func (e *Env) Network() string {
	if e == nil {
		return ""
	}
	return e.HttpServer.Network
}

func (e *Env) Addr() string {
	if e == nil {
		return ""
	}
	return e.HttpServer.Addr
}

func (e *Env) RootDir() string {
	if e == nil {
		return ""
	}
	return e.rootDir
}

func (e *Env) ConfDir() string {
	if e == nil {
		return ""
	}
	return e.confDir
}

func (e *Env) StaticDir() string {
	if e == nil {
		return ""
	}
	return resolvePath(e.rootDir, e.HttpServer.StaticDir)
}

func (e *Env) ReadTimeout() time.Duration {
	if e == nil {
		return DefaultReadTimeout
	}
	return millis(e.HttpServer.ReadTimeout, DefaultReadTimeout)
}

func (e *Env) ReadHeaderTimeout() time.Duration {
	if e == nil {
		return DefaultReadHeaderTimeout
	}
	return millis(e.HttpServer.ReadHeaderTimeout, DefaultReadHeaderTimeout)
}

func (e *Env) WriteTimeout() time.Duration {
	if e == nil {
		return DefaultWriteTimeout
	}
	return millis(e.HttpServer.WriteTimeout, DefaultWriteTimeout)
}

func (e *Env) IdleTimeout() time.Duration {
	if e == nil {
		return DefaultIdleTimeout
	}
	return millis(e.HttpServer.IdleTimeout, DefaultIdleTimeout)
}

func (e *Env) ShutdownTimeout() time.Duration {
	if e == nil {
		return DefaultShutdownTimeout
	}
	return millis(e.HttpServer.ShutdownTimeout, DefaultShutdownTimeout)
}

func (e *Env) MaxHeaderBytes() int {
	if e == nil || e.HttpServer.MaxHeaderBytes == 0 {
		return DefaultMaxHeaderBytes
	}
	return e.HttpServer.MaxHeaderBytes
}

// confFile resolves name relative to the conf directory; "" stays "" and
// absolute paths are kept.
func (e *Env) confFile(name string) string {
	if e == nil {
		return ""
	}
	return resolvePath(e.confDir, name)
}

func (e *Env) DBConfigFile() string {
	if e == nil {
		return ""
	}
	return e.confFile(e.HttpServer.DB)
}

func (e *Env) RedisConfigFile() string {
	if e == nil {
		return ""
	}
	return e.confFile(e.HttpServer.Redis)
}

func (e *Env) LoggerConfigFile() string {
	if e == nil {
		return ""
	}
	return e.confFile(e.HttpServer.Logger)
}

func (e *Env) TLS() bool {
	return e != nil && e.HttpServer.TLS
}

func (e *Env) TLSCertFile() string {
	if e == nil {
		return ""
	}
	return e.confFile(e.HttpServer.CertFile)
}

func (e *Env) TLSKeyFile() string {
	if e == nil {
		return ""
	}
	return e.confFile(e.HttpServer.KeyFile)
}

func (e *Env) EnablePprof() bool {
	return e != nil && e.HttpServer.EnablePprof
}

func (e *Env) PprofAddr() string {
	if e == nil {
		return ""
	}
	return e.HttpServer.PprofAddr
}

func (e *Env) SSETimeout() time.Duration {
	if e == nil {
		return DefaultSSETimeout
	}
	return millis(e.HttpServer.EnvSSE.Timeout, DefaultSSETimeout)
}

func (e *Env) LogRequestBody() bool {
	return e != nil && e.HttpServer.LogRequestBody
}

func (e *Env) LogResponseBody() bool {
	return e != nil && e.HttpServer.LogResponseBody
}

// ---- package-level accessors (read the default Env) -----------------------

func AppName() string                  { return Default().AppName() }
func RunMode() string                  { return Default().RunMode() }
func Network() string                  { return Default().Network() }
func Addr() string                     { return Default().Addr() }
func RootDir() string                  { return Default().RootDir() }
func ConfDir() string                  { return Default().ConfDir() }
func StaticDir() string                { return Default().StaticDir() }
func ReadTimeout() time.Duration       { return Default().ReadTimeout() }
func ReadHeaderTimeout() time.Duration { return Default().ReadHeaderTimeout() }
func WriteTimeout() time.Duration      { return Default().WriteTimeout() }
func IdleTimeout() time.Duration       { return Default().IdleTimeout() }
func ShutdownTimeout() time.Duration   { return Default().ShutdownTimeout() }
func MaxHeaderBytes() int              { return Default().MaxHeaderBytes() }
func DBConfigFile() string             { return Default().DBConfigFile() }
func RedisConfigFile() string          { return Default().RedisConfigFile() }
func LoggerConfigFile() string         { return Default().LoggerConfigFile() }
func TLS() bool                        { return Default().TLS() }
func TLSCertFile() string              { return Default().TLSCertFile() }
func TLSKeyFile() string               { return Default().TLSKeyFile() }
func EnablePprof() bool                { return Default().EnablePprof() }
func PprofAddr() string                { return Default().PprofAddr() }
func SSETimeout() time.Duration        { return Default().SSETimeout() }
func LogRequestBody() bool             { return Default().LogRequestBody() }
func LogResponseBody() bool            { return Default().LogResponseBody() }
