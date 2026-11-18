package golitekit

import (
	"context"
	"net/http"

	"github.com/hansir-hsj/GoLiteKit/logger"
)

const (
	// OK is the "status" of a successful response. Any other value is a failure.
	OK = 0
)

// Response is the JSON envelope. Status is the business code: 0 (OK) is
// success; framework errors use the HTTP code unless set with WithStatus.
type Response struct {
	Status int    `json:"status"`
	Msg    string `json:"msg"`
	Data   any    `json:"data,omitempty"`
	LogID  string `json:"logid,omitempty"`
}

// RestControllerOf is a RESTful API style generic controller with typed request body.
// Use RestController directly when no request body is needed.
type RestControllerOf[T any] struct {
	BaseControllerOf[T]
}

func (c *RestControllerOf[T]) ServeData(ctx context.Context, data any) error {
	logID := EnsureLogID(ctx)
	res := Response{
		Status: OK,
		Msg:    "OK",
		Data:   data,
		LogID:  logID,
	}
	return c.JSON(http.StatusOK, res)
}

func (c *RestControllerOf[T]) ServeOK(ctx context.Context) error {
	return c.ServeData(ctx, nil)
}

func (c *RestControllerOf[T]) ServeMsgData(ctx context.Context, msg string, data any) error {
	logID := EnsureLogID(ctx)
	res := Response{
		Status: OK,
		Msg:    msg,
		Data:   data,
		LogID:  logID,
	}
	return c.JSON(http.StatusOK, res)
}

// ServeError replies with HTTP 200 and a failure envelope. status 0 would mean
// success, so it becomes -1. Return an *AppError for a real HTTP error.
func (c *RestControllerOf[T]) ServeError(ctx context.Context, status int, msg string) error {
	if status == OK {
		status = -1
	}
	logger.AddInfo(ctx, "biz_status", status)
	logID := EnsureLogID(ctx)
	res := Response{
		Status: status,
		Msg:    msg,
		LogID:  logID,
	}
	return c.JSON(http.StatusOK, res)
}

// ServeErrorMsg is ServeError with status -1.
func (c *RestControllerOf[T]) ServeErrorMsg(ctx context.Context, msg string) error {
	return c.ServeError(ctx, -1, msg)
}

// RestController is the no-body REST controller (alias for RestControllerOf[NoBody]).
type RestController = RestControllerOf[NoBody]
