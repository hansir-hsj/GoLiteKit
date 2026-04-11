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

func (c *RestControllerOf[T]) ServeData(ctx context.Context, data any) error {
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
	return c.ServeJSON(res)
}

func (c *RestControllerOf[T]) ServeOK(ctx context.Context) error {
	return c.ServeData(ctx, nil)
}

func (c *RestControllerOf[T]) ServeMsgData(ctx context.Context, msg string, data any) error {
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
	return c.ServeJSON(res)
}

func (c *RestControllerOf[T]) ServeError(ctx context.Context, status int, msg string) error {
	logID := ""
	if tracker := GetTracker(ctx); tracker != nil {
		logID = tracker.LogID()
	}
	res := Response{
		Status: status,
		Msg:    msg,
		LogID:  logID,
	}
	return c.ServeJSON(res)
}

func (c *RestControllerOf[T]) ServeErrorMsg(ctx context.Context, msg string) error {
	return c.ServeError(ctx, -1, msg)
}

// RestController is the no-body REST controller (alias for RestControllerOf[NoBody]).
type RestController = RestControllerOf[NoBody]
