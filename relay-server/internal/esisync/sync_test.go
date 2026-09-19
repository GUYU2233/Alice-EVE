package esisync

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"relay-server/internal/esi"
	"relay-server/internal/esidata"
	"relay-server/internal/evegrant"
)

func TestRefreshClientAuthenticationModesAndRotation(t *testing.T) {
	for _, tc := range []struct {
		name, secret string
	}{{"confidential", "client-secret"}, {"public", ""}} {
		t.Run(tc.name, func(t *testing.T) {
			oldToken := "old-refresh-token"
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
					t.Fatalf("request = %s, content-type %q", r.Method, r.Header.Get("Content-Type"))
				}
				if err := r.ParseForm(); err != nil {
					t.Fatal(err)
				}
				if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != oldToken {
					t.Fatalf("form = %v", r.Form)
				}
				user, pass, basic := r.BasicAuth()
				if tc.secret != "" {
					if !basic || user != "client-id" || pass != tc.secret || r.Form.Get("client_id") != "" {
						t.Fatalf("bad confidential auth/form: %v %q %q %v", basic, user, pass, r.Form)
					}
				} else if basic || r.Form.Get("client_id") != "client-id" {
					t.Fatalf("bad public auth/form: basic=%v form=%v", basic, r.Form)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"access_token":"access-secret","refresh_token":"rotated-secret","expires_in":3600,"scope":"scope.one"}`))
			}))
			defer s.Close()
			before := time.Now().UTC()
			client := &RefreshClient{Endpoint: s.URL, ClientID: "client-id", HTTP: s.Client()}
			client.SetClientCredential(tc.secret)
			got, err := client.Refresh(context.Background(), oldToken)
			if err != nil {
				t.Fatal(err)
			}
			if got.AccessToken != "access-secret" || got.RefreshToken != "rotated-secret" || got.Scope != "scope.one" {
				t.Fatalf("grant = %+v", got)
			}
			if got.ExpiresAt.Before(before.Add(3590 * time.Second)) {
				t.Fatalf("expiry = %v", got.ExpiresAt)
			}
		})
	}
}

func TestRefreshClientInvalidGrantAndNoTokenLeakage(t *testing.T) {
	secret := "refresh-do-not-leak"
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"` + secret + `"}`))
	}))
	defer s.Close()
	_, err := (&RefreshClient{Endpoint: s.URL, ClientID: "id", HTTP: s.Client()}).Refresh(context.Background(), secret)
	if !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("refresh token leaked in error")
	}
}

type memoryJobs struct {
	mu       sync.Mutex
	jobs     []Job
	events   []string
	lastCode string
}

func (m *memoryJobs) Enqueue(_ context.Context, account, kind string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobs = append(m.jobs, Job{ID: kind, AccountID: account, Kind: kind, Status: "pending", RunAfter: at})
	return nil
}
func (m *memoryJobs) Claim(_ context.Context, now time.Time, lease time.Duration) (Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.jobs {
		j := &m.jobs[i]
		if (j.Status == "pending" || j.Status == "failed" || j.Status == "running" && !j.RunAfter.After(now)) && !j.RunAfter.After(now) {
			j.Status = "running"
			j.Attempts++
			j.ClaimToken = "claim"
			j.RunAfter = now.Add(lease)
			m.events = append(m.events, "claim")
			return *j, nil
		}
	}
	return Job{}, ErrNoJob
}
func (m *memoryJobs) finish(id, t, status, code string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.jobs {
		j := &m.jobs[i]
		if j.ID == id && j.Status == "running" && j.ClaimToken == t {
			j.Status = status
			j.ClaimToken = ""
			if !at.IsZero() {
				j.RunAfter = at
			}
			m.lastCode = code
			m.events = append(m.events, status)
			return nil
		}
	}
	return errors.New("lease lost")
}
func (m *memoryJobs) Complete(_ context.Context, id, t string) error {
	return m.finish(id, t, "succeeded", "", time.Time{})
}
func (m *memoryJobs) Reschedule(_ context.Context, id, t string, at time.Time) error {
	return m.finish(id, t, "pending", "", at)
}
func (m *memoryJobs) Retry(_ context.Context, id, t string, at time.Time, code string) error {
	return m.finish(id, t, "failed", code, at)
}
func (m *memoryJobs) Block(_ context.Context, id, t, code string) error {
	return m.finish(id, t, "blocked", code, time.Time{})
}
func (m *memoryJobs) ListAccount(_ context.Context, accountID string) ([]Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		if j.AccountID == accountID {
			out = append(out, j)
		}
	}
	return out, nil
}

func grantService(t *testing.T, account, subject, token, scope string) (*evegrant.Service, *evegrant.MemoryRepository) {
	t.Helper()
	key := base64.RawStdEncoding.EncodeToString(make([]byte, 32))
	kr, err := evegrant.ParseKeyring("test=" + key)
	if err != nil {
		t.Fatal(err)
	}
	repo := evegrant.NewMemoryRepository()
	svc, err := evegrant.NewService(repo, kr)
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Save(context.Background(), evegrant.Grant{AccountID: account, ProviderSubject: subject, RefreshToken: token, Scope: scope, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	return svc, repo
}
func gateway(t *testing.T, h http.Handler) (*esi.Gateway, *httptest.Server) {
	t.Helper()
	s := httptest.NewServer(h)
	g, err := esi.NewGateway(&esi.Client{BaseURL: s.URL, UserAgent: "esisync-test", HTTP: s.Client()})
	if err != nil {
		t.Fatal(err)
	}
	return g, s
}
func refreshServer(t *testing.T, access, refresh, scope string) (*RefreshClient, *httptest.Server) {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": access, "refresh_token": refresh, "expires_in": 3600, "scope": scope})
	}))
	return &RefreshClient{Endpoint: s.URL, ClientID: "id", HTTP: s.Client()}, s
}

func TestWorkerScopeBlockingSkipsRefreshAndESI(t *testing.T) {
	jobs := &memoryJobs{}
	_ = jobs.Enqueue(context.Background(), "a", "wallet", time.Now().Add(-time.Second))
	grants, _ := grantService(t, "a", "42", "refresh", "some.other.scope")
	w := &Worker{Jobs: jobs, Grants: grants, Refresh: &RefreshClient{}, Data: esidata.NewMemory()}
	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if jobs.jobs[0].Status != "blocked" || jobs.lastCode != "blocked_scope" {
		t.Fatalf("job=%+v code=%s", jobs.jobs[0], jobs.lastCode)
	}
}

func TestWorkerIdentityAndPrivateDispatchPersistSnapshots(t *testing.T) {
	for _, tc := range []struct {
		kind, scope, path, body string
		private                 bool
	}{
		{"identity", "", "/characters/42/", `{"character_id":42,"name":"Alice"}`, false},
		{"online", "esi-location.read_online.v1", "/characters/42/online/", `{"online":true,"last_login":"2025-01-01T00:00:00Z","last_logout":"2025-01-01T00:00:00Z","logins":1}`, true},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			jobs := &memoryJobs{}
			_ = jobs.Enqueue(context.Background(), "account", tc.kind, time.Now().Add(-time.Second))
			grants, _ := grantService(t, "account", "42", "old-refresh", tc.scope)
			refresh, rs := refreshServer(t, "access-secret", "rotated-refresh", tc.scope)
			defer rs.Close()
			g, es := gateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					t.Errorf("path=%s", r.URL.Path)
				}
				auth := r.Header.Get("Authorization")
				if tc.private && auth != "Bearer access-secret" {
					t.Errorf("auth=%q", auth)
				}
				if !tc.private && auth != "" {
					t.Errorf("public identity leaked auth: %q", auth)
				}
				w.Header().Set("ETag", `"v1"`)
				w.Header().Set("Expires", time.Now().Add(time.Hour).UTC().Format(http.TimeFormat))
				_, _ = w.Write([]byte(tc.body))
			}))
			defer es.Close()
			data := esidata.NewMemory()
			w := &Worker{Jobs: jobs, Grants: grants, Refresh: refresh, ESI: g, Data: data, Lease: time.Minute}
			if err := w.RunOnce(context.Background()); err != nil {
				t.Fatal(err)
			}
			if jobs.jobs[0].Status != "pending" {
				t.Fatalf("job=%+v", jobs.jobs[0])
			}
			snap, err := data.GetCharacterSnapshot(context.Background(), "account", 42, tc.kind)
			if err != nil {
				t.Fatal(err)
			}
			if snap.Source != "esi" || snap.ETag != `"v1"` || !json.Valid(snap.Payload) {
				t.Fatalf("snapshot=%+v", snap)
			}
			loaded, err := grants.Load(context.Background(), "account")
			if err != nil {
				t.Fatal(err)
			}
			if loaded.RefreshToken != "rotated-refresh" {
				t.Fatalf("rotation=%q", loaded.RefreshToken)
			}
		})
	}
}

func TestWorkerTransientESIFailureRetriesAndKeepsRotationEncrypted(t *testing.T) {
	jobs := &memoryJobs{}
	_ = jobs.Enqueue(context.Background(), "account", "online", time.Now().Add(-time.Second))
	grants, repo := grantService(t, "account", "42", "old-refresh", "esi-location.read_online.v1")
	refresh, rs := refreshServer(t, "access-secret", "new-refresh-secret", "esi-location.read_online.v1")
	defer rs.Close()
	g, es := gateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "temporary", http.StatusServiceUnavailable)
	}))
	defer es.Close()
	w := &Worker{Jobs: jobs, Grants: grants, Refresh: refresh, ESI: g, Data: esidata.NewMemory(), Lease: time.Minute}
	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if jobs.jobs[0].Status != "failed" || jobs.lastCode != "esi_upstream" {
		t.Fatalf("job=%+v code=%s", jobs.jobs[0], jobs.lastCode)
	}
	loaded, err := grants.Load(context.Background(), "account")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RefreshToken != "new-refresh-secret" {
		t.Fatalf("token=%q", loaded.RefreshToken)
	}
	record, err := repo.FindByAccount(context.Background(), "account")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(record.Ciphertext), "new-refresh-secret") || strings.Contains(jobs.lastCode, "new-refresh-secret") {
		t.Fatal("rotated token leaked")
	}
}

func TestWorkerInvalidGrantRevokesAndBlocks(t *testing.T) {
	jobs := &memoryJobs{}
	_ = jobs.Enqueue(context.Background(), "a", "identity", time.Now().Add(-time.Second))
	grants, _ := grantService(t, "a", "42", "bad-refresh", "")
	rs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer rs.Close()
	w := &Worker{Jobs: jobs, Grants: grants, Refresh: &RefreshClient{Endpoint: rs.URL, ClientID: "id", HTTP: rs.Client()}, Data: esidata.NewMemory()}
	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if jobs.jobs[0].Status != "blocked" || jobs.lastCode != "invalid_grant" {
		t.Fatalf("job=%+v", jobs.jobs[0])
	}
	if _, err := grants.Load(context.Background(), "a"); !errors.Is(err, evegrant.ErrNotFound) {
		t.Fatalf("grant load=%v", err)
	}
}

func TestJobRepositoryFakeLifecycleAndSchedule(t *testing.T) {
	jobs := &memoryJobs{}
	if err := ScheduleAccount(context.Background(), jobs, "account"); err != nil {
		t.Fatal(err)
	}
	if len(jobs.jobs) != len(Kinds) {
		t.Fatalf("jobs=%d", len(jobs.jobs))
	}
	job, err := jobs.Claim(context.Background(), time.Now().Add(time.Second), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "running" || job.Attempts != 1 || job.ClaimToken == "" {
		t.Fatalf("claim=%+v", job)
	}
	if err = jobs.Retry(context.Background(), job.ID, job.ClaimToken, time.Now().Add(-time.Second), "transient"); err != nil {
		t.Fatal(err)
	}
	job, err = jobs.Claim(context.Background(), time.Now(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if job.Attempts != 2 {
		t.Fatalf("attempts=%d", job.Attempts)
	}
	if err = jobs.Complete(context.Background(), job.ID, job.ClaimToken); err != nil {
		t.Fatal(err)
	}
	if jobs.jobs[0].Status != "succeeded" {
		t.Fatalf("status=%s", jobs.jobs[0].Status)
	}
}

func TestWorkerCloseStopsPromptly(t *testing.T) {
	jobs := &memoryJobs{}
	w := &Worker{Jobs: jobs, Interval: time.Hour, Lease: time.Minute}
	w.Start(context.Background())
	done := make(chan struct{})
	go func() { w.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Close did not stop worker")
	}
	w.Close()
}

var _ JobRepository = (*memoryJobs)(nil)
