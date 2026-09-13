package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"relay-server/internal/store"
)

type Envelope struct {
	Version int       `json:"v"`
	Type    string    `json:"type"`
	ID      string    `json:"id"`
	TS      time.Time `json:"ts"`
	Sender  string    `json:"sender"`
	Payload any       `json:"payload"`
}
type PairResponse struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expiresAt"`
}
type PairConfirmRequest struct {
	Code       string `json:"code"`
	DeviceName string `json:"deviceName"`
	DeviceType string `json:"deviceType"`
	PublicKey  string `json:"publicKey,omitempty"`
}
type DeviceResponse struct {
	DeviceID    string `json:"deviceId"`
	DeviceToken string `json:"deviceToken"`
}
type Server struct {
	mu      sync.Mutex
	store   store.Store
	seen    map[string]bool
	pairs   map[string]PairResponse
	mux     *http.ServeMux
	events  chan Envelope
	backlog []Envelope
}

func NewServer(st ...store.Store) *Server {
	var backend store.Store = store.NewMemory()
	if len(st) > 0 && st[0] != nil {
		backend = st[0]
	}
	s := &Server{store: backend, seen: map[string]bool{}, pairs: map[string]PairResponse{}, mux: http.NewServeMux(), events: make(chan Envelope, 32)}
	s.mux.HandleFunc("/health", s.health)
	s.mux.HandleFunc("/api/v1/pair", s.pair)
	s.mux.HandleFunc("/api/v1/pair/confirm", s.confirmPair)
	s.mux.HandleFunc("/api/v1/events", s.eventsStream)
	s.mux.HandleFunc("/api/v1/alerts", s.alerts)
	s.mux.HandleFunc("/api/v1/messages", s.publish)
	s.mux.HandleFunc("/api/v1/messages/ack", s.ack)
	s.mux.HandleFunc("/api/v1/devices/revoke", s.revoke)
	return s
}
func (s *Server) Handler() http.Handler { return s.mux }
func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
func (s *Server) pair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var raw [4]byte
	if _, err := rand.Read(raw[:]); err != nil {
		http.Error(w, "random unavailable", 500)
		return
	}
	code := fmt.Sprintf("%06d", (int64(raw[0])<<16|int64(raw[1])<<8|int64(raw[2]))%1000000)
	response := PairResponse{Code: code, ExpiresAt: time.Now().Add(5 * time.Minute).UTC()}
	_ = s.store.AddPair(r.Context(), store.PairCode{Code: code, ExpiresAt: response.ExpiresAt})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}
func (s *Server) confirmPair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	defer r.Body.Close()
	var req PairConfirmRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Code) != 6 || req.DeviceName == "" || (req.DeviceType != "desktop" && req.DeviceType != "mobile") {
		http.Error(w, "invalid pairing", 400)
		return
	}
	if _, ok := s.store.ConsumePair(r.Context(), req.Code); !ok {
		http.Error(w, "pairing code expired or not found", 401)
		return
	}
	deviceID := store.NewDeviceID()
	rawToken := store.NewDeviceID() + store.NewDeviceID()
	digest := sha256.Sum256([]byte(rawToken))
	_ = s.store.AddDevice(store.Device{ID: deviceID, TokenHash: hex.EncodeToString(digest[:]), Name: req.DeviceName, Type: req.DeviceType, PublicKey: req.PublicKey, CreatedAt: time.Now().UTC()})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(DeviceResponse{DeviceID: deviceID, DeviceToken: rawToken})
}
func (s *Server) publish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == "" || !s.store.VerifyToken(token) {
		http.Error(w, "invalid device token", 401)
		return
	}
	if r.ContentLength > 1<<20 {
		http.Error(w, "request too large", 413)
		return
	}
	defer r.Body.Close()
	var env Envelope
	if err := json.NewDecoder(r.Body).Decode(&env); err != nil || env.Version != 1 || env.ID == "" || env.Type == "" || env.Sender != "desktop" {
		http.Error(w, "invalid envelope", 400)
		return
	}
	s.mu.Lock()
	duplicate := s.seen[env.ID]
	if !duplicate {
		s.seen[env.ID] = true
	}
	s.mu.Unlock()
	if !duplicate {
		s.mu.Lock()
		s.backlog = append(s.backlog, env)
		s.mu.Unlock()
		select {
		case s.events <- env:
		default:
		}
	}
	_ = token
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"accepted": true, "duplicate": duplicate, "id": env.ID})
}
func (s *Server) ack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost { http.Error(w, "method not allowed", 405); return }
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == "" || !s.store.VerifyToken(token) { http.Error(w, "invalid device token", 401); return }
	var req struct{ ID string `json:"id"` }
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil || req.ID == "" { http.Error(w, "invalid id", 400); return }
	s.mu.Lock(); defer s.mu.Unlock()
	for i := range s.backlog { if s.backlog[i].ID == req.ID { s.backlog = append(s.backlog[:i], s.backlog[i+1:]...); break } }
	w.Header().Set("Content-Type", "application/json"); _ = json.NewEncoder(w).Encode(map[string]any{"acked": true, "id": req.ID})
}

func (s *Server) revoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var v struct {
		DeviceID string `json:"deviceId"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&v) != nil || !s.store.RevokeDevice(v.DeviceID) {
		http.Error(w, "not found", 404)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) alerts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == "" || !s.store.VerifyToken(token) {
		http.Error(w, "invalid device token", 401)
		return
	}
	s.mu.Lock()
	out := append([]Envelope(nil), s.backlog...)
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) eventsStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", 500)
		return
	}
	ready, _ := json.Marshal(Envelope{Version: 1, Type: "ready", TS: time.Now().UTC(), Sender: "relay"})
	_, _ = w.Write([]byte("data: " + string(ready) + "\n\n"))
	f.Flush()
	for {
		select {
		case env := <-s.events:
			b, _ := json.Marshal(env)
			_, _ = w.Write([]byte("data: " + string(b) + "\n\n"))
			f.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
