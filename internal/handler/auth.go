package handler

import (
	"net/http"
	"strings"

	"github.com/edmondlafaydavid/scoreplay/internal/service"
)

// APIKeyMiddleware rejects requests without a valid API key.
// Accepts key via X-API-Key header or Authorization: Bearer <key>.
func APIKeyMiddleware(svc *service.ClientService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := extractAPIKey(r)
			if key == "" {
				writeError(w, http.StatusUnauthorized, CodeAPIKeyRequired, "API key required")
				return
			}

			client, err := svc.FindByRawKey(r.Context(), key)
			if err != nil {
				writeError(w, http.StatusInternalServerError, CodeInternalError, "internal server error")
				return
			}
			if client == nil {
				writeError(w, http.StatusUnauthorized, CodeInvalidAPIKey, "invalid API key")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func extractAPIKey(r *http.Request) string {
	if v := r.Header.Get("X-API-Key"); v != "" {
		return v
	}
	if v := r.Header.Get("Authorization"); strings.HasPrefix(v, "Bearer ") {
		return strings.TrimPrefix(v, "Bearer ")
	}
	return ""
}
