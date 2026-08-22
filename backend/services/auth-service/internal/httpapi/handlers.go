package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
	mux.HandleFunc("PUT /users/{id}", h.updateUser)
	mux.HandleFunc("POST /change-password", h.changePassword)
	mux.HandleFunc("POST /users/{id}/reset-password", h.resetPassword)
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
	// MustChangePassword true = password-nya ditetapkan admin lewat reset, dan
	// frontend harus mengarahkan user mengganti sendiri sebelum memakai aplikasi.
	MustChangePassword bool `json:"must_change_password"`
}

func toUserResponse(u model.User) userResponse {
	return userResponse{
		ID: u.ID, Email: u.Email, Username: u.Username, FullName: u.FullName, Phone: u.Phone,
		IsSuperAdmin: u.IsSuperAdmin, Status: u.Status, LastLoginAt: u.LastLoginAt, CreatedAt: u.CreatedAt,
		MustChangePassword: u.MustChangePassword,
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
		`SELECT id, email, COALESCE(username, ''), password_hash, full_name, COALESCE(phone, ''), is_super_admin, status, must_change_password, last_login_at, created_at, updated_at
		 FROM users WHERE email = $1`, req.Email,
	).Scan(&u.ID, &u.Email, &u.Username, &u.PasswordHash, &u.FullName, &u.Phone, &u.IsSuperAdmin, &u.Status, &u.MustChangePassword, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt)
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
		`SELECT id, email, COALESCE(username, ''), full_name, COALESCE(phone, ''), is_super_admin, status, must_change_password, last_login_at, created_at
		 FROM users ORDER BY full_name ASC`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat daftar user")
		return
	}
	defer rows.Close()

	users := []userResponse{}
	for rows.Next() {
		var u userResponse
		if err := rows.Scan(&u.ID, &u.Email, &u.Username, &u.FullName, &u.Phone, &u.IsSuperAdmin, &u.Status, &u.MustChangePassword, &u.LastLoginAt, &u.CreatedAt); err != nil {
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

	ctx := r.Context()
	u, err := h.insertUser(ctx, req, username, string(hash))
	// Kolom username juga UNIQUE, padahal isinya cuma diturunkan dari bagian
	// sebelum "@". Dua orang dengan nama sama di domain berbeda
	// (budi@perusahaan-a.com lalu budi@perusahaan-b.com) karena itu bentrok di
	// username, bukan di email -- sebelumnya pendaftaran kedua ditolak 409
	// "Email sudah terdaftar" padahal emailnya masih bebas. Username hanya
	// dipakai untuk tampilan (login selalu lewat email), jadi bentrokan
	// diselesaikan dengan menambah akhiran acak, bukan menolak user.
	if uniqueConstraintOf(err) == "users_username_key" {
		u, err = h.insertUser(ctx, req, username+"-"+shortSuffix(), string(hash))
	}
	if err != nil {
		if uniqueConstraintOf(err) == "users_email_key" || strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, "Email sudah terdaftar")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal membuat user")
		return
	}

	h.events.Publish("auth.user.registered", newAuditEvent("auth.user.registered", "auth-service", nil, nil, "create", "user", u.ID, u))

	writeJSON(w, http.StatusCreated, toUserResponse(u))
}

type updateUserRequest struct {
	FullName string `json:"full_name"`
	Phone    string `json:"phone"`
	Status   string `json:"status"`
}

// updateUser adalah satu-satunya jalan menonaktifkan/mengunci akun lewat API.
// login & refresh sudah menghormati kolom status sejak awal, tapi sebelum ini
// status hanya bisa diubah lewat SQL langsung.
func (h *Handler) updateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.FullName = strings.TrimSpace(req.FullName)
	if req.FullName == "" {
		writeError(w, http.StatusBadRequest, "Nama lengkap wajib diisi")
		return
	}
	// Beda dari updateBranch di company-service yang menganggap status kosong
	// sebagai "active": di sini status kosong berarti TIDAK DIUBAH. Menyunting
	// nama seseorang tidak boleh diam-diam membuka kembali akun yang sengaja
	// dinonaktifkan atau dikunci.
	if req.Status != "" && req.Status != "active" && req.Status != "inactive" && req.Status != "locked" {
		writeError(w, http.StatusBadRequest, "Status harus active, inactive, atau locked")
		return
	}

	ctx := r.Context()
	var u model.User
	err := h.pool.QueryRow(ctx,
		`UPDATE users SET full_name = $1, phone = $2, status = COALESCE(NULLIF($3, ''), status), updated_at = now()
		 WHERE id = $4
		 RETURNING id, email, COALESCE(username, ''), full_name, COALESCE(phone, ''), is_super_admin, status, must_change_password, last_login_at, created_at`,
		req.FullName, req.Phone, req.Status, id,
	).Scan(&u.ID, &u.Email, &u.Username, &u.FullName, &u.Phone, &u.IsSuperAdmin, &u.Status, &u.MustChangePassword, &u.LastLoginAt, &u.CreatedAt)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "User tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memperbarui user")
		return
	}

	// Akun yang tidak lagi aktif harus kehilangan sesinya SEKARANG. Tanpa ini,
	// access token yang sudah beredar tetap dipakai sampai kedaluwarsa dan
	// refresh token-nya masih bisa memperpanjang -- padahal refresh sudah
	// memeriksa status, jadi cukup mencabut refresh token untuk memutus rantai.
	if u.Status != "active" {
		if err := h.revokeAllRefreshTokens(ctx, u.ID); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal mencabut sesi user")
			return
		}
	}

	h.events.Publish("auth.user.updated", newAuditEvent("auth.user.updated", "auth-service", nil, nil, "update", "user", u.ID, u))
	writeJSON(w, http.StatusOK, toUserResponse(u))
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// changePassword mengganti password milik pemanggil sendiri. Identitasnya
// diambil dari X-User-Id yang di-inject api-gateway dari klaim JWT -- BUKAN
// dari body, supaya tidak ada jalan mengganti password orang lain.
func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Tidak terautentikasi")
		return
	}

	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	if req.CurrentPassword == "" || len(req.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "Password lama wajib diisi dan password baru minimal 8 karakter")
		return
	}
	if req.NewPassword == req.CurrentPassword {
		writeError(w, http.StatusBadRequest, "Password baru harus berbeda dari password lama")
		return
	}

	ctx := r.Context()
	var currentHash, status string
	err := h.pool.QueryRow(ctx,
		`SELECT password_hash, status FROM users WHERE id = $1`, userID,
	).Scan(&currentHash, &status)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusUnauthorized, "User tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat data user")
		return
	}
	if status != "active" {
		writeError(w, http.StatusForbidden, "Akun tidak aktif")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(req.CurrentPassword)); err != nil {
		writeError(w, http.StatusUnauthorized, "Password lama salah")
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memproses password")
		return
	}
	// must_change_password ikut dimatikan: setelah user menggantinya sendiri,
	// password yang berlaku sudah tidak diketahui siapa pun selain dia.
	if _, err := h.pool.Exec(ctx,
		`UPDATE users SET password_hash = $1, must_change_password = false, updated_at = now() WHERE id = $2`, string(newHash), userID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan password baru")
		return
	}

	// Seluruh refresh token dicabut: ganti password harus mengusir sesi yang
	// mungkin dipegang orang lain (itu alasan paling umum orang menggantinya).
	// Pemanggil sendiri ikut terusir dan harus login lagi -- disengaja, dan
	// frontend memang mengarahkannya ke halaman login setelah ini.
	if err := h.revokeAllRefreshTokens(ctx, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal mencabut sesi lama")
		return
	}

	h.events.Publish("auth.user.password_changed", newAuditEvent("auth.user.password_changed", "auth-service", &userID, nil, "update", "user", userID, nil))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type resetPasswordRequest struct {
	// NewPassword opsional. Kosong = biarkan server yang membuatkan password
	// sementara acak, dan itu yang biasanya dipakai: password buatan orang
	// cenderung pendek dan mudah ditebak justru pada saat paling rawan.
	NewPassword string `json:"new_password"`
}

type resetPasswordResponse struct {
	Status string `json:"status"`
	// TemporaryPassword hanya diisi kalau server yang membuatkannya, dan ini
	// SATU-SATUNYA kali nilainya bisa dibaca -- yang tersimpan cuma hash-nya.
	TemporaryPassword string `json:"temporary_password,omitempty"`
}

// resetPassword: jalur "lupa password" di platform ini. Admin yang menetapkan,
// bukan tautan email, karena tidak ada satu pun komponen surat-menyurat di
// seluruh stack (lihat migrations/003_must_change_password.sql).
//
// Tiga hal terjadi sekaligus, dan ketiganya perlu:
//  1. Password diganti.
//  2. must_change_password dinyalakan -- password sementara sempat diketahui
//     admin, jadi user WAJIB menggantinya sendiri sebelum memakai aplikasi.
//  3. Seluruh refresh token dicabut -- kalau akunnya memang sedang dipakai
//     orang lain (alasan paling umum sebuah reset diminta), sesi itu harus
//     putus saat itu juga, bukan menunggu token kedaluwarsa.
func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req resetPasswordRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req) // body opsional
	}

	generated := false
	password := strings.TrimSpace(req.NewPassword)
	if password == "" {
		password = generateTemporaryPassword()
		generated = true
	} else if len(password) < 8 {
		writeError(w, http.StatusBadRequest, "Password minimal 8 karakter")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memproses password")
		return
	}

	ctx := r.Context()
	var email string
	err = h.pool.QueryRow(ctx, `
		UPDATE users SET password_hash = $1, must_change_password = true, updated_at = now()
		WHERE id = $2
		RETURNING email`, string(hash), id).Scan(&email)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "User tidak ditemukan")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal mereset password")
		return
	}

	if err := h.revokeAllRefreshTokens(ctx, id); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal mencabut sesi user")
		return
	}

	h.events.Publish("auth.user.password_reset", newAuditEvent("auth.user.password_reset", "auth-service", nil, &email, "update", "user", id, nil))

	resp := resetPasswordResponse{Status: "ok"}
	if generated {
		resp.TemporaryPassword = password
	}
	writeJSON(w, http.StatusOK, resp)
}

// generateTemporaryPassword membuat password acak yang LOLOS aturan panjang
// minimum dan tetap bisa dibacakan lewat telepon: hex 12 karakter + akhiran
// tetap yang memastikan ada huruf besar & simbol.
func generateTemporaryPassword() string {
	buf := make([]byte, 6)
	_, _ = rand.Read(buf)
	return "Reset-" + hex.EncodeToString(buf)
}

func (h *Handler) revokeAllRefreshTokens(ctx context.Context, userID string) error {
	_, err := h.pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	return err
}

func (h *Handler) insertUser(ctx context.Context, req createUserRequest, username, passwordHash string) (model.User, error) {
	var u model.User
	err := h.pool.QueryRow(ctx,
		`INSERT INTO users (email, username, password_hash, full_name, phone, status)
		 VALUES ($1, $2, $3, $4, $5, 'active')
		 RETURNING id, email, username, full_name, phone, is_super_admin, status, created_at`,
		req.Email, username, passwordHash, req.FullName, req.Phone,
	).Scan(&u.ID, &u.Email, &u.Username, &u.FullName, &u.Phone, &u.IsSuperAdmin, &u.Status, &u.CreatedAt)
	return u, err
}

// uniqueConstraintOf mengembalikan nama constraint UNIQUE yang dilanggar
// (users_email_key / users_username_key), atau string kosong kalau error-nya
// bukan pelanggaran unique.
func uniqueConstraintOf(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return pgErr.ConstraintName
	}
	return ""
}

func shortSuffix() string {
	buf := make([]byte, 3)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
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
