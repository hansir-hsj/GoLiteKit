package golite

import "context"

type Controller interface {
	Serve(ctx context.Context) error
}
