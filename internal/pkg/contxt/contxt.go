package contxt

import (
	"context"
	"os"
	"time"
)

func NewContext(timeout time.Duration) context.Context {
	if os.Getenv("CONTEXT_TEST") != "" {
		return context.Background()
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	go func() {
		<-ctx.Done()
		cancel()
	}()
	return ctx
}
