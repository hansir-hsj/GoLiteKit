// middleware demonstrates custom middleware and per-group middleware in GoLiteKit.
//
// Routes:
//
//	GET /public/ping     no auth required
//	GET /api/profile     requires X-Token header
//	GET /api/admin       requires X-Token + X-Admin header
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"

	glk "github.com/hansir-hsj/GoLiteKit"
)

// ---- middleware ------------------------------------------------------------

// RequestIDMiddleware adds a request ID header to every response.
func RequestIDMiddleware(next glk.Handler) glk.Handler {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		if logID := glk.EnsureLogID(ctx); logID != "" {
			w.Header().Set("X-Request-ID", logID)
		}
		return next(ctx, w, r)
	}
}

// AuthMiddleware rejects requests missing the X-Token header.
func AuthMiddleware(next glk.Handler) glk.Handler {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		if r.Header.Get("X-Token") == "" {
			return glk.ErrUnauthorized("missing X-Token", nil)
		}
		return next(ctx, w, r)
	}
}

// AdminMiddleware rejects requests missing the X-Admin header.
func AdminMiddleware(next glk.Handler) glk.Handler {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		if r.Header.Get("X-Admin") == "" {
			return glk.ErrForbidden("admin only", nil)
		}
		return next(ctx, w, r)
	}
}

// ---- controllers -----------------------------------------------------------

type PingController struct {
	glk.BaseController
}

func (c *PingController) Serve(ctx context.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

type ProfileController struct {
	glk.BaseController
}

func (c *ProfileController) Serve(ctx context.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"user": "alice"})
}

type AdminController struct {
	glk.BaseController
}

func (c *AdminController) Serve(ctx context.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"role": "admin"})
}

// ---- main ------------------------------------------------------------------

func main() {
	app := glk.NewApp()

	// Global middleware applied to every route.
	app.Use(RequestIDMiddleware)

	// Public routes — no auth.
	public := app.Group("/public")
	public.GET("/ping", &PingController{})

	// Authenticated routes.
	api := app.Group("/api")
	api.Use(AuthMiddleware)
	api.GET("/profile", &ProfileController{})

	// Admin-only nested group — inherits AuthMiddleware from parent.
	admin := api.Group("/admin")
	admin.Use(AdminMiddleware)
	admin.GET("", &AdminController{})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	log.Println("listening on :8080")
	if err := app.ListenAndServe(ctx, glk.ServerConfig{Addr: ":8080"}); err != nil {
		log.Fatal(err)
	}
}
