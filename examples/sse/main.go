// sse demonstrates Server-Sent Events (SSE) streaming in GoLiteKit.
//
// Routes:
//   GET /events      streams 5 events, one per second
//   GET /chat        streams named events (join, message, leave)
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	glk "github.com/hansir-hsj/GoLiteKit"
)

// ---- controllers -----------------------------------------------------------

// CounterController streams a counter event every second for 5 seconds.
type CounterController struct {
	glk.BaseController[glk.NoBody]
}

func (c *CounterController) Serve(ctx context.Context) error {
	for i := 1; i <= 5; i++ {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if err := c.SendSSEData(map[string]int{"count": i}); err != nil {
			return err
		}
		time.Sleep(time.Second)
	}
	return c.SendSSEEvent("done", "stream finished")
}

// ChatController streams a short simulated chat session using named events.
type ChatController struct {
	glk.BaseController[glk.NoBody]
}

func (c *ChatController) Serve(ctx context.Context) error {
	events := []glk.SSEvent{
		{Event: "join", Data: "alice joined the room"},
		{Event: "message", Data: map[string]string{"from": "alice", "text": "hello!"}},
		{Event: "message", Data: map[string]string{"from": "bob", "text": "hi alice"}},
		{Event: "leave", Data: "alice left the room"},
	}

	for i, ev := range events {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		ev.ID = fmt.Sprintf("%d", i+1)
		if err := c.SendSSE(ev); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil
}

// ---- main ------------------------------------------------------------------

func main() {
	app := glk.NewApp()

	app.GET("/events", &CounterController{})
	app.GET("/chat", &ChatController{})

	log.Println("listening on :8080")
	log.Println("  curl -N http://localhost:8080/events")
	log.Println("  curl -N http://localhost:8080/chat")
	if err := app.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
