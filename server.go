package golitekit

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Addr              string
	Network           string
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	MaxHeaderBytes    int
	ShutdownTimeout   time.Duration
	TLSCertFile       string
	TLSKeyFile        string
}

// DefaultServerConfig returns sensible defaults.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		Addr:              ":8080",
		Network:           "tcp",
		ReadTimeout:       30 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
		ShutdownTimeout:   10 * time.Second,
	}
}

// Server wraps http.Server with graceful shutdown.
type Server struct {
	config     ServerConfig
	mu         sync.Mutex
	httpServer *http.Server
	listener   net.Listener
	done       chan error
	started    bool
}

// NewServer creates a new Server.
func NewServer(config ServerConfig) *Server {
	defaults := DefaultServerConfig()
	if config.Network == "" {
		config.Network = defaults.Network
	}
	if config.Addr == "" {
		config.Addr = defaults.Addr
	}
	if config.ReadTimeout == 0 {
		config.ReadTimeout = defaults.ReadTimeout
	}
	if config.ReadHeaderTimeout == 0 {
		config.ReadHeaderTimeout = defaults.ReadHeaderTimeout
	}
	if config.WriteTimeout == 0 {
		config.WriteTimeout = defaults.WriteTimeout
	}
	if config.IdleTimeout == 0 {
		config.IdleTimeout = defaults.IdleTimeout
	}
	if config.MaxHeaderBytes == 0 {
		config.MaxHeaderBytes = defaults.MaxHeaderBytes
	}
	if config.ShutdownTimeout == 0 {
		config.ShutdownTimeout = defaults.ShutdownTimeout
	}
	return &Server{config: config}
}

// Start begins listening and serving without signal handling.
// Returns immediately after the listener is ready.
// Use Shutdown to stop the server.
func (s *Server) Start(handler http.Handler) error {
	_, err := s.startServing(handler)
	return err
}

// ListenAndServe starts the server and blocks until ctx is cancelled,
// then performs graceful shutdown within ShutdownTimeout.
func (s *Server) ListenAndServe(ctx context.Context, handler http.Handler) error {
	if err := s.Start(handler); err != nil {
		return err
	}

	select {
	case serveErr := <-s.Done():
		return serveErr
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.config.ShutdownTimeout)
		defer cancel()
		return s.Shutdown(shutdownCtx)
	}
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	httpServer := s.httpServer
	s.mu.Unlock()

	if httpServer == nil {
		return nil
	}
	if err := httpServer.Shutdown(ctx); err != nil {
		return err
	}

	s.mu.Lock()
	s.started = false
	s.mu.Unlock()
	return nil
}

// Addr returns the listening address.
func (s *Server) Addr() string {
	s.mu.Lock()
	listener := s.listener
	addr := s.config.Addr
	s.mu.Unlock()

	if listener != nil {
		return listener.Addr().String()
	}
	return addr
}

// Done returns a channel that receives the background Serve result after Start
// or Run begins serving. A nil value means the server stopped via Shutdown.
func (s *Server) Done() <-chan error {
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()

	if done == nil {
		ch := make(chan error)
		close(ch)
		return ch
	}
	return done
}

func (s *Server) currentListener() net.Listener {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listener
}

func (s *Server) reserveStart() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return fmt.Errorf("server already started")
	}
	s.started = true
	return nil
}

func (s *Server) releaseStart() {
	s.mu.Lock()
	s.started = false
	s.mu.Unlock()
}

func (s *Server) startServing(handler http.Handler) (<-chan error, error) {
	if err := s.reserveStart(); err != nil {
		return nil, err
	}

	httpServer := s.newHTTPServer(handler)
	ln, err := s.listen()
	if err != nil {
		s.releaseStart()
		return nil, err
	}

	s.mu.Lock()
	s.httpServer = httpServer
	s.listener = ln
	serveChan := s.serveLocked(ln)
	s.mu.Unlock()
	return serveChan, nil
}

func (s *Server) newHTTPServer(handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadTimeout:       s.config.ReadTimeout,
		WriteTimeout:      s.config.WriteTimeout,
		IdleTimeout:       s.config.IdleTimeout,
		ReadHeaderTimeout: s.config.ReadHeaderTimeout,
		MaxHeaderBytes:    s.config.MaxHeaderBytes,
	}
}

func (s *Server) listen() (net.Listener, error) {
	ln, err := net.Listen(s.config.Network, s.config.Addr)
	if err != nil {
		return nil, fmt.Errorf("listen error: %w", err)
	}

	if s.config.TLSCertFile == "" || s.config.TLSKeyFile == "" {
		return ln, nil
	}

	cert, err := tls.LoadX509KeyPair(s.config.TLSCertFile, s.config.TLSKeyFile)
	if err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("load tls cert error: %w", err)
	}
	return tls.NewListener(ln, &tls.Config{Certificates: []tls.Certificate{cert}}), nil
}

func (s *Server) serveLocked(ln net.Listener) <-chan error {
	s.done = make(chan error, 1)
	done := s.done
	httpServer := s.httpServer
	go func() {
		err := httpServer.Serve(ln)
		if err == http.ErrServerClosed {
			err = nil
		}
		s.mu.Lock()
		if s.done == done {
			s.started = false
		}
		s.mu.Unlock()
		done <- err
		close(done)
	}()
	return done
}
