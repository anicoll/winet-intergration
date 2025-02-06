package handler

import (
	"net/http"

	"go.uber.org/zap"
)

func LoggingMiddleware(next http.Handler) http.Handler {
	logger := zap.L()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Info(r.RequestURI)
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("unhandled method"))
			return
		}
		// Call the next handler, which can be another middleware in the chain, or the final handler.
		next.ServeHTTP(w, r)
	})
}
