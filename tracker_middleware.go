package golite

import (
	"context"
	"net/http"
)

func TrackerMiddleware(ctx context.Context, w http.ResponseWriter, req *http.Request, queue MiddlewareQueue) error {
	ctx = WithTracker(ctx)
	tracker := GetTracker(ctx)
	defer tracker.LogTracker(ctx)
	return queue.Next(ctx, w, req)
}
