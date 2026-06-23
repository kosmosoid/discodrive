package carddav

import "context"

const ctxRawBodyKey ctxKey = 1

// WithRawBody stores the raw PUT body in the context (raw-body middleware, Task 3).
func WithRawBody(ctx context.Context, raw []byte) context.Context {
	return context.WithValue(ctx, ctxRawBodyKey, raw)
}

func rawBody(ctx context.Context) []byte {
	if v, ok := ctx.Value(ctxRawBodyKey).([]byte); ok {
		return v
	}
	return nil
}
