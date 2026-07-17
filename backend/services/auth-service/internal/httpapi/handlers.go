package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/enterprise-digital-platform/auth-service/internal/eventbus"
	"github.com/enterprise-digital-platform/auth-service/internal/jwtutil"
	"github.com/enterprise-digital-platform/auth-service/internal/metrics"
	"github.com/enterprise-digital-platform/auth-service/internal/model"
)

type Handler struct {
	pool            *pgxpool.Pool
	events          *eventbus.Publisher
	jwtSecret       string
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

func NewHandler(pool *pgxpool.Pool, events *eventbus.Publisher, jwtSecret string, accessTTL, refreshTTL time.Duration) *Handler {
	return &Handler{pool: pool, events: events, jwtSecret: jwtSecret, accessTokenTTL: accessTTL, refreshTokenTTL: refreshTTL}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", h.health)
	mux.Handle("GET /metrics", metrics.Handler())
	mux.HandleFunc("POST /login", h.login)
	mux.HandleFunc("POST /refresh", h.refresh)
	mux.HandleFunc("POST /logout", h.logout)
	mux.HandleFunc("GET /users", h.listUsers)
	mux.HandleFunc("POST /users", h.createUser)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "auth-service"})
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userResponse struct {
	ID           string     `json:"id"`
	Email        string     `json:"email"`
	Username     string     `json:"username"`
	FullName     string     `json:"full_name"`
	Phone        string     `json:"phone"`
	IsSuperAdmin bool       `json:"is_super_admin"`
	Status       string     `json:"status"`
	LastLoginAt  *time.Time `json:"last_login_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

func toUserResponse(u model.User) userResponse {
	return userResponse{
		ID: u.ID, Email: u.Email, Username: u.Username, FullName: u.FullName, Phone: u.Phone,
		IsSuperAdmin: u.IsSuperAdmin, Status: u.Status, LastLoginAt: u.LastLoginAt, CreatedAt: u.CreatedAt,
	}
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "Email dan password wajib diisi")
		return
	}

	ctx := r.Context()
	var u model.User
	err := h.pool.QueryRow(ctx,
		`SELECT id, email, COALESCE(username, ''), password_hash, full_name, COALESCE(phone, ''), is_super_admin, status, last_login_at, created_at, updated_at
		 FROM users WHERE email = $1`, req.Email,
	).Scan(&u.ID, &u.Email, &u.Username, &u.PasswordHash, &u.FullName, &u.Phone, &u.IsSuperAdmin, &u.Status, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusUnauthorized, "Email atau password salah")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data user")
		return
	}

	if u.Status != "active" {
		writeError(w, http.StatusForbidden, "Akun tidak aktif")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "Email atau password salah")
		return
	}

	accessToken, err := jwtutil.IssueAccessToken(h.jwtSecret, u.ID, u.Email, u.FullName, u.IsSuperAdmin, h.accessTokenTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat token")
		return
	}

	refreshToken, refreshHash := generateOpaqueToken()
	if _, err := h.pool.Exec(ctx,
		`INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		u.ID, refreshHash, time.Now().Add(h.refreshTokenTTL),
	); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat refresh token")
		return
	}

	now := time.Now()
	if _, err := h.pool.Exec(ctx, `UPDATE users SET last_login_at = $1 WHERE id = $2`, now, u.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui last login")
		return
	}
	u.LastLoginAt = &now

	h.events.Publish("auth.user.logged_in", newAuditEvent("auth.user.logged_in", "auth-service", &u.ID, &u.Email, "login", "user", u.ID, nil))

	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"token_type":    "Bearer",
		"expires_in":    int(h.accessTokenTTL.Seconds()),
		"user":          toUserResponse(u),
	})
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "refresh_token wajib diisi")
		return
	}

	ctx := r.Context()
	hash := hashToken(req.RefreshToken)

	var userID string
	var expiresAt time.Time
	var revokedAt *time.Time
	err := h.pool.QueryRow(ctx,
		`SELECT user_id, expires_at, revoked_at FROM refresh_tokens WHERE token_hash = $1`, hash,
	).Scan(&userID, &expiresAt, &revokedAt)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusUnauthorized, "Refresh token tidak valid")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal validasi refresh token")
		return
	}
	if revokedAt != nil || time.Now().After(expiresAt) {
		writeError(w, http.StatusUnauthorized, "Refresh token sudah tidak berlaku")
		return
	}

	var u model.User
	err = h.pool.QueryRow(ctx,
		`SELECT id, email, full_name, is_super_admin, status FROM users WHERE id = $1`, userID,
	).Scan(&u.ID, &u.Email, &u.FullName, &u.IsSuperAdmin, &u.Status)
	if err != nil || u.Status != "active" {
		writeError(w, http.StatusUnauthorized, "User tidak ditemukan atau tidak aktif")
		return
	}

	accessToken, err := jwtutil.IssueAccessToken(h.jwtSecret, u.ID, u.Email, u.FullName, u.IsSuperAdmin, h.accessTokenTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": accessToken,
		"token_type":   "Bearer",
		"expires_in":   int(h.accessTokenTTL.Seconds()),
	})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "refresh_token wajib diisi")
		return
	}
	hash := hashToken(req.RefreshToken)
	if _, err := h.pool.Exec(r.Context(),
		`UPDATE refresh_tokens SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL`, hash,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal logout")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, email, COALESCE(username, ''), full_name, COALESCE(phone, ''), is_super_admin, status, last_login_at, created_at
		 FROM users ORDER BY full_name ASC`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat daftar user")
		return
	}
	defer rows.Close()

	users := []userResponse{}
	for rows.Next() {
		var u userResponse
		if err := rows.Scan(&u.ID, &u.Email, &u.Username, &u.FullName, &u.Phone, &u.IsSuperAdmin, &u.Status, &u.LastLoginAt, &u.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca data user")
			return
		}
		users = append(users, u)
	}
	writeJSON(w, http.StatusOK, users)
}

type createUserRequest struct {
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	Phone    string `json:"phone"`
	Password string `json:"password"`
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.FullName = strings.TrimSpace(req.FullName)
	if req.Email == "" || req.FullName == "" || len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "Email, nama lengkap wajib diisi dan password minimal 8 karakter")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memproses password")
		return
	}

	username, _, _ := strings.Cut(req.Email, "@")

	var u model.User
	ctx := r.Context()
	err = h.pool.QueryRow(ctx,
		`INSERT INTO users (email, username, password_hash, full_name, phone, status)
		 VALUES ($1, $2, $3, $4, $5, 'active')
		 RETURNING id, email, username, full_name, phone, is_super_admin, status, created_at`,
		req.Email, username, string(hash), req.FullName, req.Phone,
	).Scan(&u.ID, &u.Email, &u.Username, &u.FullName, &u.Phone, &u.IsSuperAdmin, &u.Status, &u.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "Email sudah terdaftar")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal membuat user")
		return
	}

	h.events.Publish("auth.user.registered", newAuditEvent("auth.user.registered", "auth-service", nil, nil, "create", "user", u.ID, u))

	writeJSON(w, http.StatusCreated, toUserResponse(u))
}

func generateOpaqueToken() (token string, hash string) {
	buf := make([]byte, 32)
	_, _ = rand.Read(buf)
	token = hex.EncodeToString(buf)
	return token, hashToken(token)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
