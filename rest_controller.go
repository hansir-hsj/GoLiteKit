package golitekit

const (
	OK = 0
)

type Response struct {
	Status int    `json:"status"`
	Msg    string `json:"msg,omitempty"`
	Data   any    `json:"data,omitempty"`
}

// RestController is a RESTful API style generic controller
// T is the request body type, which can be a concrete structure or NoBody
type RestController[T any] struct {
	BaseController[T]
}

func (c *RestController[T]) ServeData(data any) {
	res := Response{
		Status: OK,
		Msg:    "OK",
		Data:   data,
	}
	c.BaseController.ServeJSON(res)
}

func (c *RestController[T]) ServeError(err error) {
	res := Response{
		Status: OK,
		Msg:    err.Error(),
	}
	c.BaseController.ServeJSON(res)
}

// RestVNet Controller is a convenient alias for REST Controllers without request bodies
// Suitable for headless RESTful interfaces such as GET, DELETE, etc
type RestGetController = RestController[NoBody]
