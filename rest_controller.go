package golitekit

import "context"

const (
	OK = 0
)

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

func (c *RestControllerOf[T]) ServeData(ctx context.Context, data any) {
	logID := ""
	if tracker := GetTracker(ctx); tracker != nil {
		logID = tracker.LogID()
	}
	res := Response{
		Status: OK,
		Msg:    "OK",
		Data:   data,
		LogID:  logID,
	}
	if err := c.ServeJSON(res); err != nil {
		SetError(ctx, ErrInternal("failed to serialize response", err))
	}
}

func (c *RestControllerOf[T]) ServeOK(ctx context.Context) {
	c.ServeData(ctx, nil)
}

func (c *RestControllerOf[T]) ServeMsgData(ctx context.Context, msg string, data any) {
	logID := ""
	if tracker := GetTracker(ctx); tracker != nil {
		logID = tracker.LogID()
	}

	res := Response{
		Status: OK,
		Msg:    msg,
		Data:   data,
		LogID:  logID,
	}
	if err := c.ServeJSON(res); err != nil {
		SetError(ctx, ErrInternal("failed to serialize response", err))
	}
}

func (c *RestControllerOf[T]) ServeError(ctx context.Context, status int, msg string) {
	logID := ""
	if tracker := GetTracker(ctx); tracker != nil {
		logID = tracker.LogID()
	}

	res := Response{
		Status: status,
		Msg:    msg,
		LogID:  logID,
	}
	if err := c.ServeJSON(res); err != nil {
		SetError(ctx, ErrInternal("failed to serialize response", err))
	}
}

func (c *RestControllerOf[T]) ServeErrorMsg(ctx context.Context, msg string) {
	c.ServeError(ctx, -1, msg)
}

// RestController is the no-body REST controller (alias for RestControllerOf[NoBody]).
// Embed this for GET/DELETE endpoints that don't parse a request body.
type RestController = RestControllerOf[NoBody]
