package golite

import (
	"context"
)

func TrackerMiddleware(ctx context.Context, queue MiddlewareQueue) error {
	ctx = WithTracker(ctx)
	tracker := GetTracker(ctx)
	defer tracker.LogTracker(ctx)
	return queue.Next(ctx)
}
