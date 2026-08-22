package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"kairo/internal/database"
)

func (s *Server) SetDB(db *database.DB) {
	s.db = db
}

// isSecureRequest decides whether to set Secure on cookies.
// In production behind TLS or a terminating proxy, this is true.
// On plain http localhost development, we allow non-secure cookies so login actually works.
func isSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if r.Header.Get("X-Forwarded-Proto") == "https" {
		return true
	}
	// If host is not localhost, assume production https in front
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	// localhost / 127.0.0.1 / ::1 → allow http for dev
	if host == "localhost" || host == "127.0.0.1" {
		return false
	}
	// Default to secure in production
	return true
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiError("method not allowed"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiError("invalid request body"))
		return
	}
	if len(req.Username) > 64 || len(req.Password) > 128 {
		writeJSON(w, http.StatusBadRequest, apiError("invalid credentials"))
		return
	}

	user, err := s.db.GetUserByUsername(req.Username)
	if err != nil {
		slog.Error("login query", "error", err)
		writeJSON(w, http.StatusInternalServerError, apiError("internal error"))
		return
	}
	if user == nil || !database.CheckPassword(user.PasswordHash, req.Password) {
		writeJSON(w, http.StatusUnauthorized, apiError("invalid credentials"))
		return
	}

	ip := s.clientIP(r)
	ipStr := ""
	if ip != nil {
		ipStr = ip.String()
	}

	session, err := s.db.CreateSession(user.ID, ipStr, r.UserAgent(), time.Duration(s.cfg.SessionTTL)*time.Hour)
	if err != nil {
		slog.Error("create session", "error", err)
		writeJSON(w, http.StatusInternalServerError, apiError("internal error"))
		return
	}

	_ = s.db.UpdateUserLastLogin(user.ID)

	http.SetCookie(w, &http.Cookie{
		Name:     "kairo_session",
		Value:    session.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   s.cfg.SessionTTL * 3600,
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true,
		"user": map[string]interface{}{
			"id":       user.ID,
			"username": user.Username,
			"role":     user.Role,
		},
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiError("method not allowed"))
		return
	}

	cookie, err := r.Cookie("kairo_session")
	if err == nil {
		_ = s.db.DeleteSession(cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "kairo_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	writeJSON(w, http.StatusOK, apiMessage("logged out"))
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user := s.authenticateRequest(r)
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, apiError("not authenticated"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true,
		"user": map[string]interface{}{
			"id":         user.ID,
			"username":   user.Username,
			"role":       user.Role,
			"api_key":    user.APIKey,
			"rate_limit": user.RateLimit,
		},
	})
}
