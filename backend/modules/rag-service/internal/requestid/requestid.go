// Package requestid logs one access-log line per HTTP request tagged with
// the X-Request-Id header (set by api-gateway, see
// backend/services/api-gateway/internal/gateway/gateway.go), so a request
// can be correlated across the gateway and this service's log stream in
// Loki/Grafana. It does NOT thread the ID into every log.Printf call
// deeper in a handler (that would need every call site rewritten to carry
// a context) -- this line is enough to see "did this service receive this
// request, and when."
package requestid

import (
	"log"
	"net/http"

	"github.com/google/uuid"
)

const Header = "X-Request-Id"

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(Header)
		if id == "" {
			// Direct hit bypassing api-gateway (e.g. curl straight at this
			// service, or a test) -- still tag the line so the format is
			// consistent, just not correlated to anything upstream.
			id = uuid.NewString()
		}
		log.Printf("request_id=%s method=%s path=%s", id, r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
