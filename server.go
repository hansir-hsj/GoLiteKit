package golitekit

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
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
		Addr:            ":8080",
		Network:         "tcp",
		ReadTimeout:     30 * time.Second,
		WriteTimeout:    30 * time.Second,
		IdleTimeout:     60 * time.Second,
		MaxHeaderBytes:  1 << 20,
		ShutdownTimeout: 10 * time.Second,
	}
}

// Server wraps http.Server with graceful shutdown.
type Server struct {
	config     ServerConfig
	httpServer *http.Server
	listener   net.Listener
}

// NewServer creates a new Server.
func NewServer(config ServerConfig) *Server {
	if config.Network == "" {
		config.Network = "tcp"
	}
	if config.Addr == "" {
		config.Addr = ":8080"
	}
	if config.ShutdownTimeout == 0 {
		config.ShutdownTimeout = 10 * time.Second
	}
	return &Server{config: config}
}

// Run starts the server (blocking).
func (s *Server) Run(handler http.Handler) error {
	s.httpServer = &http.Server{
		Handler:           handler,
		ReadTimeout:       s.config.ReadTimeout,
		WriteTimeout:      s.config.WriteTimeout,
		IdleTimeout:       s.config.IdleTimeout,
		ReadHeaderTimeout: s.config.ReadHeaderTimeout,
		MaxHeaderBytes:    s.config.MaxHeaderBytes,
	}

	ln, err := net.Listen(s.config.Network, s.config.Addr)
	if err != nil {
		return fmt.Errorf("listen error: %w", err)
	}

	if s.config.TLSCertFile != "" && s.config.TLSKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(s.config.TLSCertFile, s.config.TLSKeyFile)
		if err != nil {
			return fmt.Errorf("load tls cert error: %w", err)
		}
		ln = tls.NewListener(ln, &tls.Config{Certificates: []tls.Certificate{cert}})
		fmt.Printf("Server listening on https://%s\n", s.config.Addr)
	} else {
		fmt.Printf("Server listening on http://%s\n", s.config.Addr)
	}

	s.listener = ln

	errChan := make(chan error, 1)
	go func() {
		if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
		close(errChan)
	}()

	s.waitForShutdown()
	return <-errChan
}

func (s *Server) waitForShutdown() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	fmt.Fprintf(os.Stderr, "\n%s received signal %v, shutting down...\n", time.Now().Format(time.RFC3339), sig)

	ctx, cancel := context.WithTimeout(context.Background(), s.config.ShutdownTimeout)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Server shutdown error: %v\n", err)
	}
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}

// Addr returns the listening address.
func (s *Server) Addr() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.config.Addr
}
