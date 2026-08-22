package server

import (
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"kairo/internal/database"
)

// handleAPI routes authenticated /api/* requests. Supports both the legacy
// single API key and per-user API keys from the database.
func (s *Server) handleAPI(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/"), "/")

	// Auth routes don't require authentication - use global limiter for login brute force
	if path == "auth/login" {
		if !s.apiLimiter.Allow() {
			if s.Metrics != nil {
				s.Metrics.APIRateLimited.Inc()
			}
			writeJSON(w, http.StatusTooManyRequests, apiError("rate limit exceeded"))
			return
		}
		s.handleLogin(w, r)
		return
	}

	// Check for session auth (cookie) first, then fall back to API key
	user := s.authenticateRequest(r)
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, apiError("invalid or missing authentication"))
		return
	}
	// Per-user rate limiting; 0 means unlimited
	if !s.allowAPI(user) {
		if s.Metrics != nil {
			s.Metrics.APIRateLimited.Inc()
		}
		writeJSON(w, http.StatusTooManyRequests, apiError("rate limit exceeded"))
		return
	}

	// Route based on path prefix
	switch {
	case path == "auth/logout":
		s.handleLogout(w, r)
	case path == "auth/me":
		s.handleMe(w, r)
	case path == "users" && r.Method == http.MethodGet:
		s.requireAdmin(s.handleListUsers)(w, r)
	case path == "users" && r.Method == http.MethodPost:
		s.requireAdmin(s.handleCreateUser)(w, r)
	case strings.HasPrefix(path, "users/") && strings.HasSuffix(path, "/devices"):
		s.requireAdmin(s.handleUserDevices)(w, r)
	case strings.HasPrefix(path, "users/") && strings.HasSuffix(path, "/api-key/regenerate"):
		s.requireAdmin(s.handleRegenerateAPIKey)(w, r)
	case strings.HasPrefix(path, "users/"):
		s.requireAdmin(s.handleDeleteUser)(w, r)
	case path == "devices" && r.Method == http.MethodGet:
		s.requireAdmin(s.handleAllDevices)(w, r)
	case path == "me/devices":
		s.handleMyDevices(w, r)
	case path == "me/api-key/regenerate":
		s.handleMyRegenerateAPIKey(w, r)
	case path == "me/traffic":
		s.handleMyTraffic(w, r)
	case path == "traffic" && r.Method == http.MethodGet:
		s.requireAdmin(s.handleTraffic)(w, r)
	case strings.HasPrefix(path, "users/") && strings.HasSuffix(path, "/rate-limit"):
		s.requireAdmin(s.handleUpdateUserRateLimit)(w, r)
	case path == "public-config":
		s.handlePublicConfig(w, r)
	case path == "domain/check":
		s.handleDomainCheck(w, r)
	case path == "domain/request" && r.Method == http.MethodPost:
		s.handleDomainRequest(w, r)
	case path == "domain/requests" && r.Method == http.MethodGet:
		s.requireAdmin(s.handleListDomainRequests)(w, r)
	case strings.HasPrefix(path, "domain/requests/") && strings.HasSuffix(path, "/approve"):
		s.requireAdmin(s.handleApproveDomainRequest)(w, r)
	case strings.HasPrefix(path, "domain/requests/") && strings.HasSuffix(path, "/reject"):
		s.requireAdmin(s.handleRejectDomainRequest)(w, r)
	case path == "allow":
		s.handleAllow(w, r)
	case path == "me/ips":
		s.handleMyIPs(w, r)
	case strings.HasPrefix(path, "users/") && strings.HasSuffix(path, "/ips"):
		s.handleUserIPs(w, r)
	case path == "restricted":
		s.requireAdmin(s.handleRestricted)(w, r)
	case path == "generate":
		s.requireAdmin(s.handleGenerate)(w, r)
	case path == "status":
		s.requireAdmin(s.handleAPIStatus)(w, r)
	default:
		writeJSON(w, http.StatusNotFound, apiError("unknown endpoint"))
	}
}

// authenticateRequest checks for session cookie first, then falls back to API key.
func (s *Server) authenticateRequest(r *http.Request) *database.User {
	// Try session cookie first (requires DB)
	if s.db != nil {
		if cookie, err := r.Cookie("kairo_session"); err == nil {
			if session, _ := s.db.GetSession(cookie.Value); session != nil {
				if user, _ := s.db.GetUserByID(session.UserID); user != nil {
					return user
				}
			}
		}
	}

	// Fall back to API key (legacy single key or per-user key)
	key := r.URL.Query().Get("key")
	if key == "" {
		key = r.Header.Get("X-API-Key")
	}
	if key == "" {
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			key = strings.TrimPrefix(auth, "Bearer ")
		}
	}

	if key == "" {
		return nil
	}

	// Check legacy single API key - treat as admin (requires cfg)
	if s.cfg != nil && subtle.ConstantTimeCompare([]byte(key), []byte(s.cfg.APIKey)) == 1 {
		return &database.User{
			ID:       0,
			Username: "admin",
			Role:     "admin",
			APIKey:   key,
		}
	}

	// Check per-user API keys (requires DB)
	if s.db != nil {
		if user, _ := s.db.GetUserByAPIKey(key); user != nil {
			return user
		}
	}

	return nil
}

func (s *Server) handleAllow(w http.ResponseWriter, r *http.Request) {
	user := s.authenticateRequest(r)
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, apiError("not authenticated"))
		return
	}
	// GET is admin-only (global allowlist is sensitive)
	if r.Method == http.MethodGet {
		if user.Role != "admin" {
			writeJSON(w, http.StatusForbidden, apiError("admin access required"))
			return
		}
		writeJSON(w, http.StatusOK, apiData(s.st.AllowedList()))
		return
	}

	ipParam := r.URL.Query().Get("ip")
	if ipParam == "" {
		writeJSON(w, http.StatusBadRequest, apiError("missing 'ip' query parameter"))
		return
	}
	ip := net.ParseIP(ipParam)
	if ip == nil {
		writeJSON(w, http.StatusBadRequest, apiError("invalid IP address"))
		return
	}
	if ip.IsLoopback() {
		writeJSON(w, http.StatusBadRequest, apiError("loopback addresses are always allowed"))
		return
	}

	// Non-admin can only allowlist their own current IP
	if user.Role != "admin" {
		clientIP := s.clientIP(r)
		if clientIP == nil || ip.String() != clientIP.String() {
			writeJSON(w, http.StatusForbidden, apiError("you can only allowlist your own current IP; ask admin for other IPs"))
			return
		}
		// For DELETE, non-admin is not allowed at all (global list)
		if r.Method == http.MethodDelete {
			writeJSON(w, http.StatusForbidden, apiError("admin access required"))
			return
		}
	}

	switch r.Method {
	case http.MethodPost:
		// Per-user self-add: store per-user first, enforce limit, then ensure global
		if user.Role != "admin" {
			if exists, _ := s.db.IsIPAllowlistedForUser(user.ID, ip.String()); exists {
				writeJSON(w, http.StatusConflict, apiError("IP already allowlisted for you"))
				return
			}
			if user.IpLimit != 0 {
				cnt, _ := s.db.CountUserIPs(user.ID)
				if cnt >= user.IpLimit {
					writeJSON(w, http.StatusForbidden, apiError("IP limit reached"))
					return
				}
			}
			if err := s.db.AddUserIP(user.ID, ip.String()); err != nil && !strings.Contains(err.Error(), "UNIQUE") {
				writeJSON(w, http.StatusInternalServerError, apiError(err.Error()))
				return
			}
			// ensure global allowlist also has it for DNS (union)
			_, _ = s.st.AddAllowed(ip)
			writeJSON(w, http.StatusOK, apiMessage("IP allowlisted"))
			return
		}
		// admin: add globally
		added, err := s.st.AddAllowed(ip)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, apiError(err.Error()))
			return
		}
		if !added {
			writeJSON(w, http.StatusConflict, apiError("IP already allowlisted"))
			return
		}
		writeJSON(w, http.StatusOK, apiMessage("IP allowlisted"))
	case http.MethodDelete:
		// Double-check admin for DELETE (already handled above for non-admin)
		if user.Role != "admin" {
			writeJSON(w, http.StatusForbidden, apiError("admin access required"))
			return
		}
		removed, err := s.st.RemoveAllowed(ip)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, apiError(err.Error()))
			return
		}
		if !removed {
			writeJSON(w, http.StatusNotFound, apiError("IP is not allowlisted"))
			return
		}
		// also remove from all per-user tables for completeness (admin delete for all)
		_, _ = s.db.RemoveUserIPAny(ip.String())
		writeJSON(w, http.StatusOK, apiMessage("IP removed from allowlist"))
	default:
		writeJSON(w, http.StatusMethodNotAllowed, apiError("method not allowed"))
	}
}

func (s *Server) handleMyIPs(w http.ResponseWriter, r *http.Request) {
	user := s.authenticateRequest(r)
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, apiError("not authenticated"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		ips, err := s.db.ListUserIPs(user.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, apiError(err.Error()))
			return
		}
		if ips == nil {
			ips = []database.UserAllowedIP{}
		}
		// include limit info
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "ips": ips, "limit": user.IpLimit, "count": len(ips)})
		return
	case http.MethodPost:
		ipParam := r.URL.Query().Get("ip")
		if ipParam == "" {
			// try JSON body
			var req struct{ IP string `json:"ip"` }
			_ = json.NewDecoder(r.Body).Decode(&req)
			ipParam = req.IP
		}
		if ipParam == "" {
			writeJSON(w, http.StatusBadRequest, apiError("missing ip"))
			return
		}
		ip := net.ParseIP(ipParam)
		if ip == nil {
			writeJSON(w, http.StatusBadRequest, apiError("invalid IP"))
			return
		}
		if ip.IsLoopback() {
			writeJSON(w, http.StatusBadRequest, apiError("loopback not needed"))
			return
		}
		// check limit
		if user.IpLimit != 0 {
			cnt, _ := s.db.CountUserIPs(user.ID)
			if cnt >= user.IpLimit {
				writeJSON(w, http.StatusForbidden, apiError("IP limit reached"))
				return
			}
		}
		if err := s.db.AddUserIP(user.ID, ip.String()); err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				writeJSON(w, http.StatusConflict, apiError("IP already added"))
			} else {
				writeJSON(w, http.StatusInternalServerError, apiError(err.Error()))
			}
			return
		}
		// also add to global allowlist for DNS (union)
		_, _ = s.st.AddAllowed(ip)
		writeJSON(w, http.StatusOK, apiMessage("IP added"))
		return
	case http.MethodDelete:
		ipParam := r.URL.Query().Get("ip")
		if ipParam == "" {
			writeJSON(w, http.StatusBadRequest, apiError("missing ip"))
			return
		}
		ip := net.ParseIP(ipParam)
		if ip == nil {
			writeJSON(w, http.StatusBadRequest, apiError("invalid IP"))
			return
		}
		removed, err := s.db.RemoveUserIP(user.ID, ip.String())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, apiError(err.Error()))
			return
		}
		if !removed {
			writeJSON(w, http.StatusNotFound, apiError("IP not found for user"))
			return
		}
		// do not automatically remove from global if other user still has it
		// check if any user still has this IP
		if ok, _ := s.db.IsIPAllowlistedAny(ip.String()); !ok {
			_, _ = s.st.RemoveAllowed(ip)
		}
		writeJSON(w, http.StatusOK, apiMessage("IP removed"))
		return
	default:
		writeJSON(w, http.StatusMethodNotAllowed, apiError("method not allowed"))
	}
}

func (s *Server) handleUserIPs(w http.ResponseWriter, r *http.Request) {
	user := s.authenticateRequest(r)
	if user == nil || user.Role != "admin" {
		writeJSON(w, http.StatusForbidden, apiError("admin required"))
		return
	}
	// path: /api/users/:id/ips
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// ["api","users",":id","ips"]
	if len(parts) < 4 {
		writeJSON(w, http.StatusBadRequest, apiError("invalid path"))
		return
	}
	id, err := strconv.Atoi(parts[2])
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiError("invalid user id"))
		return
	}
	target, err := s.db.GetUserByID(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError(err.Error()))
		return
	}
	if target == nil {
		writeJSON(w, http.StatusNotFound, apiError("user not found"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		ips, err := s.db.ListUserIPs(id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, apiError(err.Error()))
			return
		}
		if ips == nil {
			ips = []database.UserAllowedIP{}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "ips": ips, "limit": target.IpLimit, "count": len(ips)})
		return
	case http.MethodPost:
		ipParam := r.URL.Query().Get("ip")
		if ipParam == "" {
			var req struct{ IP string `json:"ip"` }
			_ = json.NewDecoder(r.Body).Decode(&req)
			ipParam = req.IP
		}
		if ipParam == "" {
			writeJSON(w, http.StatusBadRequest, apiError("missing ip"))
			return
		}
		ip := net.ParseIP(ipParam)
		if ip == nil {
			writeJSON(w, http.StatusBadRequest, apiError("invalid IP"))
			return
		}
		if target.IpLimit != 0 {
			cnt, _ := s.db.CountUserIPs(id)
			if cnt >= target.IpLimit {
				writeJSON(w, http.StatusForbidden, apiError("user IP limit reached"))
				return
			}
		}
		if err := s.db.AddUserIP(id, ip.String()); err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				writeJSON(w, http.StatusConflict, apiError("IP already exists"))
			} else {
				writeJSON(w, http.StatusInternalServerError, apiError(err.Error()))
			}
			return
		}
		_, _ = s.st.AddAllowed(ip)
		writeJSON(w, http.StatusOK, apiMessage("IP added for user"))
		return
	case http.MethodDelete:
		ipParam := r.URL.Query().Get("ip")
		if ipParam == "" {
			writeJSON(w, http.StatusBadRequest, apiError("missing ip"))
			return
		}
		ip := net.ParseIP(ipParam)
		if ip == nil {
			writeJSON(w, http.StatusBadRequest, apiError("invalid IP"))
			return
		}
		removed, err := s.db.RemoveUserIP(id, ip.String())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, apiError(err.Error()))
			return
		}
		if !removed {
			writeJSON(w, http.StatusNotFound, apiError("IP not found"))
			return
		}
		if ok, _ := s.db.IsIPAllowlistedAny(ip.String()); !ok {
			_, _ = s.st.RemoveAllowed(ip)
		}
		writeJSON(w, http.StatusOK, apiMessage("IP removed for user"))
		return
	default:
		writeJSON(w, http.StatusMethodNotAllowed, apiError("method not allowed"))
	}
}

func (s *Server) handleRestricted(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, apiData(s.st.RestrictedList()))
		return
	}

	domain := r.URL.Query().Get("domain")
	if domain == "" {
		writeJSON(w, http.StatusBadRequest, apiError("missing 'domain' query parameter"))
		return
	}

	switch r.Method {
	case http.MethodPost:
		added, err := s.st.AddRestricted(domain)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, apiError(err.Error()))
			return
		}
		if !added {
			writeJSON(w, http.StatusConflict, apiError("domain already restricted"))
			return
		}
		writeJSON(w, http.StatusOK, apiMessage("domain restricted"))
	case http.MethodDelete:
		removed, err := s.st.RemoveRestricted(domain)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, apiError(err.Error()))
			return
		}
		if !removed {
			writeJSON(w, http.StatusNotFound, apiError("domain is not restricted"))
			return
		}
		writeJSON(w, http.StatusOK, apiMessage("domain unrestricted"))
	default:
		writeJSON(w, http.StatusMethodNotAllowed, apiError("method not allowed"))
	}
}

func (s *Server) handleGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiError("method not allowed"))
		return
	}
	added, failed, err := s.st.GenerateIPs()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":         true,
		"added":      added,
		"unresolved": failed,
		"total":      s.st.AllowedCount(),
		"message":    "allowlist regenerated",
	})
}

func (s *Server) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":                 true,
		"version":            s.Version,
		"host":               s.cfg.Host,
		"admin_url":          s.cfg.EffectiveAdminURL(),
		"doh_url":            s.cfg.EffectiveDoHURL(),
		"vps_ip":             s.cfg.VPSIP,
		"uptime_seconds":     int64(time.Since(s.start).Seconds()),
		"allowlisted":        s.st.AllowedList(),
		"restricted":         s.st.RestrictedList(),
		"upstream_dns":       s.cfg.Upstream,
		"ip_source":          s.st.IPSourcePath(),
		"ip_source_interval": s.cfg.IPSource.Interval,
	})
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.db.ListUsers()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError(err.Error()))
		return
	}

	type userJSON struct {
		ID        int        `json:"id"`
		Username  string     `json:"username"`
		APIKey    string     `json:"api_key"`
		Role      string     `json:"role"`
		RateLimit int        `json:"rate_limit"`
		CreatedAt time.Time  `json:"created_at"`
		LastLogin *time.Time `json:"last_login,omitempty"`
	}

	var result []userJSON
	for _, u := range users {
		result = append(result, userJSON{
			ID:        u.ID,
			Username:  u.Username,
			APIKey:    u.APIKey,
			Role:      u.Role,
			RateLimit: u.RateLimit,
			CreatedAt: u.CreatedAt,
			LastLogin: u.LastLogin,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":   true,
		"users": result,
	})
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiError("method not allowed"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var req struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		RateLimit *int   `json:"rate_limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiError("invalid request body"))
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if len(req.Username) < 3 || len(req.Username) > 32 {
		writeJSON(w, http.StatusBadRequest, apiError("username must be 3-32 characters"))
		return
	}
	if len(req.Password) < 6 || len(req.Password) > 128 {
		writeJSON(w, http.StatusBadRequest, apiError("password must be 6-128 characters"))
		return
	}
	for _, ch := range req.Username {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '.' || ch == '_' || ch == '-') {
			writeJSON(w, http.StatusBadRequest, apiError("username contains invalid characters"))
			return
		}
	}
	rateLimit := 100
	if req.RateLimit != nil {
		rateLimit = *req.RateLimit
		if rateLimit < 0 {
			writeJSON(w, http.StatusBadRequest, apiError("rate_limit cannot be negative"))
			return
		}
		if rateLimit > 10000 {
			writeJSON(w, http.StatusBadRequest, apiError("rate_limit too large (max 10000, 0 for unlimited)"))
			return
		}
		// 0 means unlimited
	}
	user, err := s.db.CreateUserWithRateLimit(req.Username, req.Password, "user", rateLimit)
	if err != nil {
		writeJSON(w, http.StatusConflict, apiError(err.Error()))
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"ok": true,
		"user": map[string]interface{}{
			"id":        user.ID,
			"username":  user.Username,
			"api_key":   user.APIKey,
			"role":      user.Role,
			"rate_limit": user.RateLimit,
		},
	})
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, apiError("method not allowed"))
		return
	}

	idStr := strings.TrimPrefix(strings.TrimSuffix(r.URL.Path, "/"), "/api/users/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiError("invalid user id"))
		return
	}

	target, err := s.db.GetUserByID(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError(err.Error()))
		return
	}
	if target == nil {
		writeJSON(w, http.StatusNotFound, apiError("user not found"))
		return
	}
	if target.Role == "admin" {
		writeJSON(w, http.StatusForbidden, apiError("cannot delete admin user"))
		return
	}

	if err := s.db.DeleteUserAtomic(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError(err.Error()))
		return
	}

	writeJSON(w, http.StatusOK, apiMessage("user deleted"))
}

func (s *Server) handleUserDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, apiError("method not allowed"))
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/users/")
	path = strings.TrimSuffix(path, "/devices")
	id, err := strconv.Atoi(path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiError("invalid user id"))
		return
	}

	devices, err := s.db.GetDevicesByUser(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError(err.Error()))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"devices": devices,
	})
}

func (s *Server) handleRegenerateAPIKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiError("method not allowed"))
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/users/")
	path = strings.TrimSuffix(path, "/api-key/regenerate")
	id, err := strconv.Atoi(path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiError("invalid user id"))
		return
	}

	newKey, err := s.db.UpdateUserAPIKey(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError(err.Error()))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":     true,
		"api_key": newKey,
	})
}

func (s *Server) handleAllDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := s.db.GetAllDevices()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError(err.Error()))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"devices": devices,
	})
}

func (s *Server) handleMyDevices(w http.ResponseWriter, r *http.Request) {
	user := s.authenticateRequest(r)
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, apiError("not authenticated"))
		return
	}

	devices, err := s.db.GetDevicesByUser(user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError(err.Error()))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"devices": devices,
	})
}

func (s *Server) handleMyRegenerateAPIKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiError("method not allowed"))
		return
	}

	user := s.authenticateRequest(r)
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, apiError("not authenticated"))
		return
	}

	newKey, err := s.db.UpdateUserAPIKey(user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError(err.Error()))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"api_key": newKey,
	})
}

func (s *Server) handleMyTraffic(w http.ResponseWriter, r *http.Request) {
	user := s.authenticateRequest(r)
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, apiError("not authenticated"))
		return
	}
	devices, _ := s.db.GetDevicesByUser(user.ID)
	logs, _ := s.db.GetConnectionLogsByUser(user.ID)
	// Count by device and recent
	recent := logs
	if len(recent) > 20 {
		recent = recent[:20]
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":              true,
		"user_id":         user.ID,
		"devices":         len(devices),
		"total_requests":  len(logs),
		"recent":          recent,
		"rate_limit":      user.RateLimit,
		"unlimited":       user.RateLimit == 0,
	})
}

func (s *Server) handleTraffic(w http.ResponseWriter, r *http.Request) {
	users, _ := s.db.ListUsers()
	devices, _ := s.db.GetAllDevices()
	var totalLogs int
	_ = s.db.CountConnectionLogs(&totalLogs)
	logs, _ := s.db.GetRecentConnectionLogs(20)
	// per-user breakdown
	type userTraffic struct {
		Username string `json:"username"`
		Devices  int    `json:"devices"`
		Requests int    `json:"requests"`
	}
	var breakdown []userTraffic
	for _, u := range users {
		devs, _ := s.db.GetDevicesByUser(u.ID)
		cls, _ := s.db.GetConnectionLogsByUser(u.ID)
		breakdown = append(breakdown, userTraffic{Username: u.Username, Devices: len(devs), Requests: len(cls)})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":               true,
		"total_users":      len(users),
		"total_devices":    len(devices),
		"total_requests":   totalLogs,
		"allowlisted":      s.st.AllowedCount(),
		"restricted":       s.st.RestrictedCount(),
		"uptime_seconds":   int64(time.Since(s.start).Seconds()),
		"version":          s.Version,
		"recent":           logs,
		"per_user":         breakdown,
	})
}

func (s *Server) handleUpdateUserRateLimit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPatch {
		writeJSON(w, http.StatusMethodNotAllowed, apiError("method not allowed"))
		return
	}
	// path: /api/users/:id/rate-limit
	prefix := "/api/users/"
	suffix := "/rate-limit"
	idStr := strings.TrimPrefix(strings.TrimSuffix(r.URL.Path, suffix), prefix)
	idStr = strings.TrimSuffix(idStr, "/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiError("invalid user id"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	var req struct {
		RateLimit *int `json:"rate_limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RateLimit == nil {
		writeJSON(w, http.StatusBadRequest, apiError("rate_limit is required"))
		return
	}
	rl := *req.RateLimit
	if rl < 0 || rl > 10000 {
		writeJSON(w, http.StatusBadRequest, apiError("rate_limit must be 0 (unlimited) to 10000"))
		return
	}
	target, err := s.db.GetUserByID(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError(err.Error()))
		return
	}
	if target == nil {
		writeJSON(w, http.StatusNotFound, apiError("user not found"))
		return
	}
	if err := s.db.UpdateUserRateLimit(id, rl); err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError(err.Error()))
		return
	}
	// clear cached limiter so new limit takes effect
	s.userLimiters.Delete(id)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "rate_limit": rl, "unlimited": rl == 0})
}

func (s *Server) handlePublicConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":        true,
		"admin_url": s.cfg.EffectiveAdminURL(),
		"doh_url":   s.cfg.EffectiveDoHURL(),
		"host":      s.cfg.Host,
		"vps_ip":    s.cfg.VPSIP,
	})
}

func (s *Server) handleDomainCheck(w http.ResponseWriter, r *http.Request) {
	domain := strings.TrimSpace(r.URL.Query().Get("domain"))
	if domain == "" {
		writeJSON(w, http.StatusBadRequest, apiError("missing domain"))
		return
	}
	if len(domain) > 253 {
		writeJSON(w, http.StatusBadRequest, apiError("domain too long"))
		return
	}
	restricted := s.st.IsRestricted(domain)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "restricted": restricted, "domain": domain})
}

func (s *Server) handleDomainRequest(w http.ResponseWriter, r *http.Request) {
	user := s.authenticateRequest(r)
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, apiError("not authenticated"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	var req struct {
		Domain string `json:"domain"`
	}
	// Try JSON body first, then query param fallback
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Domain = r.URL.Query().Get("domain")
	}
	domain := strings.TrimSpace(req.Domain)
	domain = strings.ToLower(domain)
	if domain == "" {
		writeJSON(w, http.StatusBadRequest, apiError("domain is required"))
		return
	}
	if len(domain) < 3 || len(domain) > 253 {
		writeJSON(w, http.StatusBadRequest, apiError("invalid domain length"))
		return
	}
	if strings.Contains(domain, " ") || strings.Contains(domain, "/") {
		writeJSON(w, http.StatusBadRequest, apiError("invalid domain format"))
		return
	}
	if s.st.IsRestricted(domain) {
		writeJSON(w, http.StatusConflict, apiError("domain already proxied"))
		return
	}
	dr, err := s.db.CreateDomainRequest(user.ID, domain)
	if err != nil {
		writeJSON(w, http.StatusConflict, apiError(err.Error()))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"ok": true, "request": dr})
}

func (s *Server) handleListDomainRequests(w http.ResponseWriter, r *http.Request) {
	reqs, err := s.db.ListDomainRequests()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError(err.Error()))
		return
	}
	if reqs == nil {
		reqs = []database.DomainRequest{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "requests": reqs})
}

func (s *Server) handleApproveDomainRequest(w http.ResponseWriter, r *http.Request) {
	// path: /api/domain/requests/:id/approve
	prefix := "/api/domain/requests/"
	suffix := "/approve"
	idStr := strings.TrimPrefix(strings.TrimSuffix(r.URL.Path, suffix), prefix)
	idStr = strings.TrimSuffix(idStr, "/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiError("invalid request id"))
		return
	}
	dr, err := s.db.GetDomainRequest(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError(err.Error()))
		return
	}
	if dr == nil {
		writeJSON(w, http.StatusNotFound, apiError("request not found"))
		return
	}
	if _, err := s.st.AddRestricted(dr.Domain); err != nil {
		// if already exists, still approve
		if !strings.Contains(err.Error(), "already") {
			writeJSON(w, http.StatusBadRequest, apiError(err.Error()))
			return
		}
	}
	_ = s.db.UpdateDomainRequestStatus(id, "approved")
	writeJSON(w, http.StatusOK, apiMessage("domain approved and proxied"))
}

func (s *Server) handleRejectDomainRequest(w http.ResponseWriter, r *http.Request) {
	prefix := "/api/domain/requests/"
	suffix := "/reject"
	idStr := strings.TrimPrefix(strings.TrimSuffix(r.URL.Path, suffix), prefix)
	idStr = strings.TrimSuffix(idStr, "/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiError("invalid request id"))
		return
	}
	dr, err := s.db.GetDomainRequest(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError(err.Error()))
		return
	}
	if dr == nil {
		writeJSON(w, http.StatusNotFound, apiError("request not found"))
		return
	}
	_ = s.db.UpdateDomainRequestStatus(id, "rejected")
	writeJSON(w, http.StatusOK, apiMessage("request rejected"))
}

// requireAdmin wraps a handler with admin role check. The user must already
// be authenticated via the session/API key check above.
func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := s.authenticateRequest(r)
		if user == nil || user.Role != "admin" {
			writeJSON(w, http.StatusForbidden, apiError("admin access required"))
			return
		}
		next(w, r)
	}
}

func apiError(msg string) map[string]interface{} {
	return map[string]interface{}{"ok": false, "error": msg}
}

func apiMessage(msg string) map[string]interface{} {
	return map[string]interface{}{"ok": true, "message": msg}
}

func apiData(data []string) map[string]interface{} {
	return map[string]interface{}{"ok": true, "data": data}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
