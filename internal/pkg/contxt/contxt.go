package contxt

import (
	"context"
	"time"
)

func NewContext(timeout time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	go func() {
		<-ctx.Done()
		cancel()
	}()
	return ctx
}
