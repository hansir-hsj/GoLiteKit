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

// RestController is a RESTful API style generic controller
// T is the request body type, which can be a concrete structure or NoBody
type RestController[T any] struct {
	BaseController[T]
}

func (c *RestController[T]) ServeData(ctx context.Context, data any) {
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
	c.BaseController.ServeJSON(res)
}

func (c *RestController[T]) ServeOK(ctx context.Context) {
	c.ServeData(ctx, nil)
}

func (c *RestController[T]) ServeMsgData(ctx context.Context, msg string, data any) {
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
	c.BaseController.ServeJSON(res)
}

func (c *RestController[T]) ServeError(ctx context.Context, status int, msg string) {
	logID := ""
	if tracker := GetTracker(ctx); tracker != nil {
		logID = tracker.LogID()
	}

	res := Response{
		Status: status,
		Msg:    msg,
		LogID:  logID,
	}
	c.BaseController.ServeJSON(res)
}

func (c *RestController[T]) ServeErrorMsg(ctx context.Context, msg string) {
	c.ServeError(ctx, -1, msg)
}

// RestGetController is a convenient alias for REST Controllers without request bodies
// Suitable for headless RESTful interfaces such as GET, DELETE, etc
type RestGetController = RestController[NoBody]
