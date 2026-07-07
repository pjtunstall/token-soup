package chat

import "net/http"

// WithCORS wraps the API so browsers can call it from another origin (e.g. Vite dev server).
func WithCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This allows any origin by echoing it back unconditionally:
		// `w.Header().Set("Access-Control-Allow-Origin", origin)`. In
		// production, the origin should only be echoed back if it matches that
		// of our own frontend. I'm assuming here that backend and frontend
		// would actually have different origins. That was my plan. An
		// alternative would be same-origin by proxy, in which case CORS would
		// not be needed.
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
