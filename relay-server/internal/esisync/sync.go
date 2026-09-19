// Package esisync refreshes EVE OAuth grants and persists account-scoped ESI snapshots.
package esisync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"relay-server/internal/esi"
	"relay-server/internal/esidata"
	"relay-server/internal/evegrant"
)

var ErrInvalidGrant = errors.New("eve oauth refresh grant is invalid")

type RefreshedGrant struct {
	AccessToken, RefreshToken, Scope string
	ExpiresAt                        time.Time
}
type RefreshClient struct {
	Endpoint, ClientID string
	HTTP               *http.Client
	credential         string
}

func (c *RefreshClient) SetClientCredential(value string) { c.credential = value }

func (c *RefreshClient) Refresh(ctx context.Context, refreshToken string) (RefreshedGrant, error) {
	if c == nil || strings.TrimSpace(c.Endpoint) == "" || strings.TrimSpace(c.ClientID) == "" || refreshToken == "" {
		return RefreshedGrant{}, errors.New("oauth refresh client is not configured")
	}
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refreshToken}}
	if c.credential == "" {
		form.Set("client_id", c.ClientID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return RefreshedGrant{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if c.credential != "" {
		req.SetBasicAuth(c.ClientID, c.credential)
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return RefreshedGrant{}, err
	}
	defer resp.Body.Close()
	var raw struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		Scope        string `json:"scope"`
		Error        string `json:"error"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 64<<10))
	_ = dec.Decode(&raw)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if raw.Error == "invalid_grant" {
			return RefreshedGrant{}, ErrInvalidGrant
		}
		return RefreshedGrant{}, fmt.Errorf("oauth refresh failed: HTTP %d", resp.StatusCode)
	}
	if raw.AccessToken == "" {
		return RefreshedGrant{}, errors.New("oauth refresh returned no access token")
	}
	if raw.RefreshToken == "" {
		raw.RefreshToken = refreshToken
	}
	return RefreshedGrant{AccessToken: raw.AccessToken, RefreshToken: raw.RefreshToken, Scope: strings.TrimSpace(raw.Scope), ExpiresAt: time.Now().UTC().Add(time.Duration(raw.ExpiresIn) * time.Second)}, nil
}

type Job struct {
	ID, AccountID, Kind, Status, ClaimToken string
	LastErrorCode                           string
	RunAfter                                time.Time
	Attempts                                int
}
type JobRepository interface {
	Enqueue(context.Context, string, string, time.Time) error
	Claim(context.Context, time.Time, time.Duration) (Job, error)
	Complete(context.Context, string, string) error
	Reschedule(context.Context, string, string, time.Time) error
	Retry(context.Context, string, string, time.Time, string) error
	Block(context.Context, string, string, string) error
	ListAccount(context.Context, string) ([]Job, error)
}

var ErrNoJob = errors.New("no sync job ready")

var RequiredScopes = map[string]string{
	"identity": "", "online": "esi-location.read_online.v1", "location": "esi-location.read_location.v1", "ship": "esi-location.read_ship_type.v1", "skills": "esi-skills.read_skills.v1", "skillqueue": "esi-skills.read_skillqueue.v1", "wallet": "esi-wallet.read_character_wallet.v1", "wallet_journal": "esi-wallet.read_character_wallet.v1", "transactions": "esi-wallet.read_character_wallet.v1", "assets": "esi-assets.read_assets.v1", "orders": "esi-markets.read_character_orders.v1", "order_history": "esi-markets.read_character_orders.v1", "killmails": "esi-killmails.read_killmails.v1", "notifications": "esi-characters.read_notifications.v1",
	"clones": "esi-clones.read_clones.v1", "implants": "esi-clones.read_implants.v1", "fatigue": "esi-characters.read_fatigue.v1", "loyalty": "esi-characters.read_loyalty.v1", "standings": "esi-characters.read_standings.v1", "titles": "esi-characters.read_titles.v1", "blueprints": "esi-characters.read_blueprints.v1", "industry_jobs": "esi-industry.read_character_jobs.v1", "mining": "esi-industry.read_character_mining.v1", "contracts": "esi-contracts.read_character_contracts.v1", "calendar": "esi-calendar.read_calendar_events.v1", "contacts": "esi-characters.read_contacts.v1", "contact_labels": "esi-characters.read_contacts.v1", "mail_labels": "esi-mail.read_mail.v1", "mail_headers": "esi-mail.read_mail.v1", "research": "esi-characters.read_agents_research.v1", "fw_stats": "esi-characters.read_fw_stats.v1", "medals": "esi-characters.read_medals.v1", "fittings": "esi-fittings.read_fittings.v1",
}
var Kinds = []string{"identity", "online", "location", "ship", "skills", "skillqueue", "wallet", "wallet_journal", "transactions", "assets", "orders", "order_history", "killmails", "notifications", "clones", "implants", "fatigue", "loyalty", "standings", "titles", "blueprints", "industry_jobs", "mining", "contracts", "calendar", "contacts", "contact_labels", "mail_labels", "mail_headers", "research", "fw_stats", "medals", "fittings"}

func ScheduleAccount(ctx context.Context, r JobRepository, accountID string) error {
	for _, k := range Kinds {
		if err := r.Enqueue(ctx, accountID, k, time.Now().UTC()); err != nil {
			return err
		}
	}
	return nil
}
func hasScope(all, want string) bool {
	if want == "" {
		return true
	}
	for _, s := range strings.Fields(all) {
		if s == want {
			return true
		}
	}
	return false
}

type Worker struct {
	Jobs            JobRepository
	Grants          *evegrant.Service
	Refresh         *RefreshClient
	ESI             *esi.Gateway
	Data            esidata.Repository
	Interval, Lease time.Duration
	cancel          context.CancelFunc
	wg              sync.WaitGroup
}

func (w *Worker) Start(parent context.Context) {
	if w.Interval <= 0 {
		w.Interval = time.Second
	}
	if w.Lease <= 0 {
		w.Lease = time.Minute
	}
	ctx, cancel := context.WithCancel(parent)
	w.cancel = cancel
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		t := time.NewTicker(w.Interval)
		defer t.Stop()
		for {
			_ = w.RunOnce(ctx)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}
func (w *Worker) withAccessToken(ctx context.Context, accountID string, fn func(string, int64) (any, error)) (any, error) {
	grant, err := w.Grants.Load(ctx, accountID)
	if err != nil {
		return nil, err
	}
	refreshed, err := w.Refresh.Refresh(ctx, grant.RefreshToken)
	if err != nil {
		return nil, err
	}
	id, err := strconv.ParseInt(grant.ProviderSubject, 10, 64)
	if err != nil || id <= 0 {
		return nil, errors.New("invalid subject")
	}
	return fn(refreshed.AccessToken, id)
}
func (w *Worker) ContractItems(ctx context.Context, accountID string, contractID int64) (any, error) {
	return w.withAccessToken(ctx, accountID, func(tok string, id int64) (any, error) {
		v, _, e := w.ESI.CharacterContractItems(ctx, id, contractID, tok)
		return v, e
	})
}
func (w *Worker) ContractBids(ctx context.Context, accountID string, contractID int64) (any, error) {
	return w.withAccessToken(ctx, accountID, func(tok string, id int64) (any, error) {
		v, _, e := w.ESI.CharacterContractBids(ctx, id, contractID, tok)
		return v, e
	})
}
func (w *Worker) MailBody(ctx context.Context, accountID string, mailID int64) (any, error) {
	return w.withAccessToken(ctx, accountID, func(tok string, id int64) (any, error) {
		v, _, e := w.ESI.CharacterMailBody(ctx, id, mailID, tok)
		return v, e
	})
}
func (w *Worker) KillmailDetail(ctx context.Context, killmailID int64, hash string) (any, error) {
	v, _, e := w.ESI.KillmailDetail(ctx, killmailID, hash)
	return v, e
}

func (w *Worker) Close() {
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
}
func (w *Worker) RunOnce(ctx context.Context) error {
	job, err := w.Jobs.Claim(ctx, time.Now().UTC(), w.Lease)
	if errors.Is(err, ErrNoJob) {
		return nil
	}
	if err != nil {
		return err
	}
	grant, err := w.Grants.Load(ctx, job.AccountID)
	if err != nil {
		return w.Jobs.Retry(ctx, job.ID, job.ClaimToken, time.Now().Add(time.Minute), "grant_unavailable")
	}
	required, known := RequiredScopes[job.Kind]
	if !known {
		return w.Jobs.Block(ctx, job.ID, job.ClaimToken, "unknown_kind")
	}
	if !hasScope(grant.Scope, required) {
		return w.Jobs.Block(ctx, job.ID, job.ClaimToken, "blocked_scope")
	}
	refreshed, err := w.Refresh.Refresh(ctx, grant.RefreshToken)
	if errors.Is(err, ErrInvalidGrant) {
		_ = w.Grants.Revoke(ctx, job.AccountID)
		return w.Jobs.Block(ctx, job.ID, job.ClaimToken, "invalid_grant")
	}
	if err != nil {
		return w.Jobs.Retry(ctx, job.ID, job.ClaimToken, time.Now().Add(backoff(job.Attempts)), "refresh_failed")
	}
	scope := refreshed.Scope
	if scope == "" {
		scope = grant.Scope
	}
	if refreshed.RefreshToken != grant.RefreshToken {
		if err = w.Grants.Save(ctx, evegrant.Grant{AccountID: grant.AccountID, ProviderSubject: grant.ProviderSubject, RefreshToken: refreshed.RefreshToken, ExpiresAt: refreshed.ExpiresAt, Scope: scope}); err != nil {
			return w.Jobs.Retry(ctx, job.ID, job.ClaimToken, time.Now().Add(time.Minute), "grant_rotate_failed")
		}
	}
	characterID, err := strconv.ParseInt(grant.ProviderSubject, 10, 64)
	if err != nil || characterID <= 0 {
		return w.Jobs.Block(ctx, job.ID, job.ClaimToken, "invalid_subject")
	}
	payload, meta, err := w.fetch(ctx, job.Kind, characterID, refreshed.AccessToken)
	if err != nil {
		code := "esi_failed"
		var esiErr *esi.Error
		if errors.As(err, &esiErr) {
			switch {
			case esiErr.Status == 401:
				code = "esi_unauthorized"
			case esiErr.Status == 403:
				code = "esi_forbidden"
			case esiErr.Status == 404:
				code = "esi_not_found"
			case esiErr.Status == 420 || esiErr.Status == 429:
				code = "esi_rate_limited"
			case esiErr.Status >= 500:
				code = "esi_upstream"
			default:
				code = "esi_" + string(esiErr.Kind)
			}
		}
		return w.Jobs.Retry(ctx, job.ID, job.ClaimToken, time.Now().Add(backoff(job.Attempts)), code)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	// ESI may encode an empty optional collection as JSON null. Snapshot storage
	// deliberately accepts only objects/arrays, and list domains are represented
	// consistently as [] so clients can distinguish an empty result from absence.
	if string(body) == "null" {
		body = []byte("[]")
	}
	now := time.Now().UTC()
	exp := meta.ExpiresAt
	if exp.Before(now) {
		exp = now.Add(5 * time.Minute)
	}
	if err = w.Data.UpsertCharacterSnapshot(ctx, esidata.CharacterSnapshot{AccountID: job.AccountID, CharacterID: characterID, Domain: job.Kind, Payload: body, FetchedAt: now, ExpiresAt: exp, Source: "esi", ETag: meta.ETag}); err != nil {
		return w.Jobs.Retry(ctx, job.ID, job.ClaimToken, time.Now().Add(time.Minute), "store_failed")
	}
	refreshAt := exp
	// Respect upstream cache metadata while avoiding a tight loop for endpoints
	// that return an already-expired or very short TTL.
	if refreshAt.Before(now.Add(time.Minute)) {
		refreshAt = now.Add(5 * time.Minute)
	}
	return w.Jobs.Reschedule(ctx, job.ID, job.ClaimToken, refreshAt)
}
func backoff(attempt int) time.Duration {
	d := time.Duration(1<<min(attempt, 6)) * time.Minute
	return d
}
func (w *Worker) fetch(ctx context.Context, k string, id int64, tok string) (any, esi.Response, error) {
	switch k {
	case "identity":
		v, m, e := w.ESI.CharacterIdentity(ctx, id)
		return v, m, e
	case "online":
		v, m, e := w.ESI.CharacterOnline(ctx, id, tok)
		return v, m, e
	case "location":
		v, m, e := w.ESI.CharacterLocation(ctx, id, tok)
		return v, m, e
	case "ship":
		v, m, e := w.ESI.CharacterShip(ctx, id, tok)
		return v, m, e
	case "skills":
		v, m, e := w.ESI.CharacterSkills(ctx, id, tok)
		return v, m, e
	case "skillqueue":
		v, m, e := w.ESI.CharacterSkillQueue(ctx, id, tok)
		return v, m, e
	case "wallet":
		v, m, e := w.ESI.CharacterWallet(ctx, id, tok)
		return map[string]any{"balance": v}, m, e
	case "wallet_journal":
		v, m, e := w.ESI.CharacterWalletJournal(ctx, id, tok)
		return v, m, e
	case "assets":
		v, m, e := w.ESI.CharacterAssets(ctx, id, tok)
		return v, m, e
	case "orders":
		v, m, e := w.ESI.CharacterOrders(ctx, id, tok, false)
		return v, m, e
	case "killmails":
		v, m, e := w.ESI.CharacterKillmails(ctx, id, tok, 1)
		return v, m, e
	case "notifications":
		v, m, e := w.ESI.CharacterNotifications(ctx, id, tok)
		return v, m, e
	case "transactions":
		v, m, e := w.ESI.CharacterWalletTransactions(ctx, id, tok)
		return v, m, e
	case "order_history":
		v, m, e := w.ESI.CharacterOrders(ctx, id, tok, true)
		return v, m, e
	case "clones":
		v, m, e := w.ESI.CharacterClones(ctx, id, tok)
		return v, m, e
	case "implants":
		v, m, e := w.ESI.CharacterImplants(ctx, id, tok)
		return v, m, e
	case "fatigue":
		v, m, e := w.ESI.CharacterFatigue(ctx, id, tok)
		return v, m, e
	case "loyalty":
		v, m, e := w.ESI.CharacterLoyalty(ctx, id, tok)
		return v, m, e
	case "standings":
		v, m, e := w.ESI.CharacterStandings(ctx, id, tok)
		return v, m, e
	case "titles":
		v, m, e := w.ESI.CharacterTitles(ctx, id, tok)
		return v, m, e
	case "blueprints":
		v, m, e := w.ESI.CharacterBlueprints(ctx, id, tok)
		return v, m, e
	case "industry_jobs":
		v, m, e := w.ESI.CharacterIndustryJobs(ctx, id, tok)
		return v, m, e
	case "mining":
		v, m, e := w.ESI.CharacterMining(ctx, id, tok)
		return v, m, e
	case "contracts":
		v, m, e := w.ESI.CharacterContracts(ctx, id, tok)
		return v, m, e
	case "calendar":
		v, m, e := w.ESI.CharacterCalendar(ctx, id, tok)
		return v, m, e
	case "contacts":
		v, m, e := w.ESI.CharacterContacts(ctx, id, tok)
		return v, m, e
	case "contact_labels":
		v, m, e := w.ESI.CharacterContactLabels(ctx, id, tok)
		return v, m, e
	case "mail_labels":
		v, m, e := w.ESI.CharacterMailLabels(ctx, id, tok)
		return v, m, e
	case "mail_headers":
		v, m, e := w.ESI.CharacterMailHeaders(ctx, id, tok)
		return v, m, e
	case "research":
		v, m, e := w.ESI.CharacterResearch(ctx, id, tok)
		return v, m, e
	case "fw_stats":
		v, m, e := w.ESI.CharacterFWStats(ctx, id, tok)
		return v, m, e
	case "medals":
		v, m, e := w.ESI.CharacterMedals(ctx, id, tok)
		return v, m, e
	case "fittings":
		v, m, e := w.ESI.CharacterFittings(ctx, id, tok)
		return v, m, e
	}
	return nil, esi.Response{}, errors.New("unknown kind")
}
