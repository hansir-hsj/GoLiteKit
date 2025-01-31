package golite

const (
	OK = 0
)

type Response struct {
	Status int    `json:"status"`
	Msg    string `json:"msg,omitempty"`
	Data   any    `json:"data,omitempty"`
}

type RestController struct {
	BaseController
}

func (c *RestController) ServeData(data any) {
	res := Response{
		Status: OK,
		Msg:    "OK",
		Data:   data,
	}
	c.BaseController.ServeJSON(res)
}
