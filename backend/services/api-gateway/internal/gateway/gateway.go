package gateway

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"github.com/enterprise-digital-platform/api-gateway/internal/config"
	"github.com/enterprise-digital-platform/api-gateway/internal/metrics"
)

// publicRoutes tidak memerlukan Authorization header (login & refresh token
// harus bisa dipanggil sebelum client punya access token).
var publicRoutes = map[string]bool{
	"POST /api/auth/login":   true,
	"POST /api/auth/refresh": true,
}

type route struct {
	prefix string // mis. "/api/auth"
	proxy  *httputil.ReverseProxy
}

func New(cfg *config.Config) http.Handler {
	routes := []route{
		{prefix: "/api/auth", proxy: newProxy(cfg.AuthServiceURL, "/api/auth")},
		{prefix: "/api/company", proxy: newProxy(cfg.CompanyServiceURL, "/api/company")},
		{prefix: "/api/rbac", proxy: newProxy(cfg.RBACServiceURL, "/api/rbac")},
		{prefix: "/api/audit", proxy: newProxy(cfg.AuditServiceURL, "/api/audit")},
		{prefix: "/api/finance", proxy: newProxy(cfg.FinanceServiceURL, "/api/finance")},
		{prefix: "/api/hr", proxy: newProxy(cfg.HRServiceURL, "/api/hr")},
		{prefix: "/api/sales", proxy: newProxy(cfg.SalesServiceURL, "/api/sales")},
		{prefix: "/api/purchasing", proxy: newProxy(cfg.PurchasingServiceURL, "/api/purchasing")},
		{prefix: "/api/warehouse", proxy: newProxy(cfg.WarehouseServiceURL, "/api/warehouse")},
		{prefix: "/api/production", proxy: newProxy(cfg.ProductionServiceURL, "/api/production")},
		{prefix: "/api/qc", proxy: newProxy(cfg.QCServiceURL, "/api/qc")},
		{prefix: "/api/asset", proxy: newProxy(cfg.AssetServiceURL, "/api/asset")},
		{prefix: "/api/ai-bi", proxy: newProxy(cfg.AIBIServiceURL, "/api/ai-bi")},
		{prefix: "/api/iot", proxy: newProxy(cfg.IoTServiceURL, "/api/iot")},
		{prefix: "/api/dw", proxy: newProxy(cfg.DWServiceURL, "/api/dw")},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"api-gateway"}`))
	})
	mux.Handle("GET /metrics", metrics.Handler())

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		for _, rt := range routes {
			if strings.HasPrefix(r.URL.Path, rt.prefix+"/") || r.URL.Path == rt.prefix {
				handleRoute(cfg, rt, w, r)
				return
			}
		}
		http.NotFound(w, r)
	})

	return withCORS(cfg.CORSAllowedOrigin, metrics.Middleware(mux))
}

func newProxy(target, stripPrefix string) *httputil.ReverseProxy {
	targetURL, err := url.Parse(target)
	if err != nil {
		log.Fatalf("api-gateway: invalid target URL %q: %v", target, err)
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(targetURL)
			path := strings.TrimPrefix(pr.In.URL.Path, stripPrefix)
			if path == "" {
				path = "/"
			}
			pr.Out.URL.Path = path
		},
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("api-gateway: proxy error for %s: %v", r.URL.Path, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"Service tujuan tidak dapat dihubungi"}`))
	}
	return proxy
}

func handleRoute(cfg *config.Config, rt route, w http.ResponseWriter, r *http.Request) {
	if !publicRoutes[requestKey(r)] {
		claims, err := authenticate(cfg.JWTSecret, r)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "Token tidak valid atau kedaluwarsa")
			return
		}
		r.Header.Set("X-User-Id", claims.Subject)
		r.Header.Set("X-User-Email", claims.Email)
		if claims.IsSuperAdmin {
			r.Header.Set("X-Is-Super-Admin", "true")
		} else {
			r.Header.Set("X-Is-Super-Admin", "false")
		}
	}
	rt.proxy.ServeHTTP(w, r)
}

func requestKey(r *http.Request) string {
	return r.Method + " " + r.URL.Path
}

type claims struct {
	Email        string `json:"email"`
	IsSuperAdmin bool   `json:"is_super_admin"`
	jwt.RegisteredClaims
}

func authenticate(secret string, r *http.Request) (*claims, error) {
	header := r.Header.Get("Authorization")
	tokenString, ok := strings.CutPrefix(header, "Bearer ")
	if !ok || tokenString == "" {
		return nil, jwt.ErrTokenMalformed
	}

	c := &claims{}
	token, err := jwt.ParseWithClaims(tokenString, c, func(t *jwt.Token) (any, error) {
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}
	return c, nil
}

func withCORS(allowedOrigin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Untuk dev lokal, Vite bisa naik di port berbeda (3000, 3001, 3002, ...)
		// kalau port default terpakai proses lain -- izinkan origin localhost apa
		// pun. Selain localhost, pakai CORS_ALLOWED_ORIGIN yang dikonfigurasi.
		origin := r.Header.Get("Origin")
		if strings.HasPrefix(origin, "http://localhost:") || strings.HasPrefix(origin, "http://127.0.0.1:") {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		} else {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Vary", "Origin")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":"` + message + `"}`))
}
