package server

import (
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"
)

// handleAPI routes authenticated /api/* requests.
func (s *Server) handleAPI(w http.ResponseWriter, r *http.Request) {
	if !s.apiLimiter.Allow() {
		if s.Metrics != nil {
			s.Metrics.APIRateLimited.Inc()
		}
		writeJSON(w, http.StatusTooManyRequests, apiError("rate limit exceeded"))
		return
	}
	if !s.checkKey(r) {
		writeJSON(w, http.StatusUnauthorized, apiError("invalid or missing API key"))
		return
	}

	path := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/"), "/")
	switch path {
	case "allow":
		s.handleAllow(w, r)
	case "restricted":
		s.handleRestricted(w, r)
	case "generate":
		s.handleGenerate(w, r)
	case "status":
		s.handleAPIStatus(w)
	default:
		writeJSON(w, http.StatusNotFound, apiError("unknown endpoint"))
	}
}

// checkKey compares the supplied key in constant time, so nobody learns about
// it from response timing. Query param, header, or Bearer token, take your pick.
func (s *Server) checkKey(r *http.Request) bool {
	key := r.URL.Query().Get("key")
	if key == "" {
		key = r.Header.Get("X-API-Key")
	}
	if key == "" {
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			key = strings.TrimPrefix(auth, "Bearer ")
		}
	}
	return subtle.ConstantTimeCompare([]byte(key), []byte(s.cfg.APIKey)) == 1
}

func (s *Server) handleAllow(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
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

	switch r.Method {
	case http.MethodPost:
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
		removed, err := s.st.RemoveAllowed(ip)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, apiError(err.Error()))
			return
		}
		if !removed {
			writeJSON(w, http.StatusNotFound, apiError("IP is not allowlisted"))
			return
		}
		writeJSON(w, http.StatusOK, apiMessage("IP removed from allowlist"))
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

// handleGenerate runs the ip-source resolution on demand. Same as -gen-ips,
// minus the reboot.
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

func (s *Server) handleAPIStatus(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":                 true,
		"version":            s.Version,
		"host":               s.cfg.Host,
		"vps_ip":             s.cfg.VPSIP,
		"uptime_seconds":     int64(time.Since(s.start).Seconds()),
		"allowlisted":        s.st.AllowedList(),
		"restricted":         s.st.RestrictedList(),
		"upstream_dns":       s.cfg.Upstream,
		"ip_source":          s.st.IPSourcePath(),
		"ip_source_interval": s.cfg.IPSource.Interval,
	})
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
