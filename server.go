package golite

import (
	"context"
	"github/hsj/golite/env"
	"github/hsj/golite/logger"
	"log"
	"net/http"
	"time"
)

type Server struct {
	addr   string
	router Router
}

func New(conf string) *Server {
	router := NewRouter()

	if err := env.Init(conf); err != nil {
		return nil
	}

	return &Server{
		addr:   env.Addr(),
		router: router,
	}
}

func (s *Server) Start() error {
	server := http.Server{
		Addr:         s.addr,
		ReadTimeout:  env.ReadTimeout(),
		WriteTimeout: env.WriteTimeout(),
		IdleTimeout:  env.IdleTimeout(),
		Handler:      s,
	}
	return server.ListenAndServe()
}

func (s *Server) OnGet(path string, controller Controller) {
	s.router.OnGet(path, controller)
}

func (s *Server) OnPost(path string, controller Controller) {
	s.router.OnPost(path, controller)
}

func (s *Server) OnPut(path string, controller Controller) {
	s.router.OnPut(path, controller)
}

func (s *Server) OnDelete(path string, controller Controller) {
	s.router.OnDelete(path, controller)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ctx := logger.WithContext(req.Context())

	logInst, err := logger.NewLogger(ctx, env.ConfDir())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	panicLogInst, err := logger.NewPanicLogger(ctx, env.ConfDir())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	ctx = WithContext(ctx)
	gcx := GetContext(ctx)
	gcx.SetContextOptions(WithRequest(req), WithResponseWriter(w), WithLogger(logInst), WithPanicLogger(panicLogInst))

	logger.AddInfo(ctx, "method", gcx.request.Method)
	logger.AddInfo(ctx, "url", gcx.request.URL)

	controller, params, ok := s.router.Route(req.Method, req.URL.Path)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if params != nil {
		gcx.SetContextOptions(WithRouterParams(params))
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	doneChan := make(chan struct{}, 1)
	panicChan := make(chan any, 1)

	go func() {
		defer func() {
			if p := recover(); p != nil {
				panicLogInst.Report(ctx, p)
				panicChan <- p
			}
		}()
		ctx = WithTracker(ctx)
		tracker := GetTracker(ctx)
		gcx = GetContext(ctx)
		logInst := gcx.Logger()

		err := controller.Init(ctx)
		if err != nil {
			return
		}
		err = controller.Serve(ctx)
		if err != nil {
			return
		}
		err = controller.Finalize(ctx)
		if err != nil {
			return
		}
		tracker.LogTracker(ctx)
		logInst.Info(ctx, "ok")

		doneChan <- struct{}{}
	}()

	select {
	case p := <-panicChan:
		log.Printf("%v", p)
	case <-ctx.Done():
		log.Print("timeout")
	case <-doneChan:
	}
}
