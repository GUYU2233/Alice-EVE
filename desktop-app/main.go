package main

import (
	"context"
	"errors"
	"eve-assistant/desktop-app/internal/app"
	"eve-assistant/desktop-app/internal/eve"
	"eve-assistant/desktop-app/internal/evesde"
	"eve-assistant/desktop-app/internal/protocol"
	"eve-assistant/desktop-app/internal/storage"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

// officialRelayURL is injected into official builds with -ldflags. It is not a
// credential; keeping the deployment hostname out of tracked source preserves
// the repository's public redaction policy.
var officialRelayURL string

type App struct {
	service    *app.ApplicationService
	relay      *app.RelayClient
	pairing    *app.PairingManager
	store      storage.Store
	localAgent *app.LocalAgent
	sde        *eve.SDEIndex
	esi        *eve.ESIClient
	realtime   *app.RealtimeClient
	chatLogs   *app.ChatLogWatcher
	gameLogs   *app.GameLogWatcher
	ingestion  *app.LocalIngestion
	close      func() error
	once       sync.Once
	startupErr string
}

// NewApp keeps the historical no-argument constructor for tests and callers
// that explicitly provide dependencies through NewAppWithStorage.
func NewApp() *App { return NewAppWithStorage(nil, nil) }

func NewAppWithStorage(store storage.Store, secrets storage.SecretStore) *App {
	relay := app.NewRelayClientWithSecrets(secrets)
	workspaceRoot, _ := os.Getwd()
	controller := app.NewLocalDesktopController("desktop", workspaceRoot)
	result := &App{service: app.NewApplicationService(), relay: relay, pairing: app.NewPairingManager(store, relay), store: store, localAgent: app.NewLocalAgent(controller, nil), sde: eve.NewSDEIndex(""), esi: eve.NewESIClient(), realtime: app.NewRealtimeClient(relay, store), chatLogs: app.NewChatLogWatcher(store), gameLogs: app.NewGameLogWatcher(store)}
	if store != nil {
		result.ingestion, _ = app.NewLocalIngestion(store)
	}
	return result
}

func NewPersistentApp() (*App, error) {
	store, secrets, closeDB, err := newPersistentDependencies()
	if err != nil {
		return nil, err
	}
	a := NewAppWithStorage(store, secrets)
	if configDir, cfgErr := os.UserConfigDir(); cfgErr == nil {
		sdeDir := filepath.Join(configDir, "Alice-EVE", "sde")
		cacheDir := filepath.Join(configDir, "Alice-EVE", "sde-cache")
		a.sde = eve.NewSDEIndex(cacheDir)
		dbPath := filepath.Join(sdeDir, "eve-sde.sqlite")
		if _, openErr := a.sde.OpenDatabase(context.Background(), dbPath); openErr != nil {
			if info, statErr := os.Stat(sdeDir); statErr == nil && info.IsDir() {
				if setErr := a.sde.SetDirectory(sdeDir); setErr == nil {
					_, _ = a.sde.Reindex(context.Background())
				}
			}
		}
	}
	a.close = closeDB
	return a, nil
}

func (a *App) StartupError() string         { return a.startupErr }
func (a *App) SetRelayURL(url string) error { return a.relay.SetURL(url) }
func (a *App) BeginOAuthLogin(deviceName string) (map[string]interface{}, error) {
	start, verifier, err := a.relay.BeginOAuthPKCE(context.Background(), deviceName)
	if err != nil {
		return nil, err
	}
	_ = verifier // retained only inside RelayClient; never expose the PKCE verifier to JavaScript.
	return map[string]interface{}{"authorizationUrl": start.AuthorizationURL, "expiresIn": start.ExpiresIn}, nil
}
func (a *App) PollOAuthLogin() (map[string]interface{}, error) {
	credentials, err := a.relay.CompletePendingOAuthPKCE(context.Background())
	if err != nil {
		if app.IsOAuthPending(err) {
			return map[string]interface{}{"pending": true}, nil
		}
		return nil, err
	}
	return map[string]interface{}{"accountId": credentials.AccountID, "deviceId": credentials.DeviceID, "accessExpiresAt": credentials.AccessExpiresAt, "refreshExpiresAt": credentials.RefreshExpiresAt}, nil
}
func (a *App) CompleteOAuthLogin(state, code, verifier string) (map[string]interface{}, error) {
	credentials, err := a.relay.CompleteOAuthPKCE(context.Background(), state, code, verifier)
	if err != nil {
		if app.IsOAuthPending(err) {
			return map[string]interface{}{"pending": true}, nil
		}
		return nil, err
	}
	return map[string]interface{}{"accountId": credentials.AccountID, "deviceId": credentials.DeviceID, "accessExpiresAt": credentials.AccessExpiresAt, "refreshExpiresAt": credentials.RefreshExpiresAt}, nil
}
func (a *App) RefreshOAuthToken() error {
	_, err := a.relay.RefreshToken(context.Background())
	return err
}
func (a *App) AuthorizeDevice(accountID, deviceName string) (map[string]interface{}, error) {
	credentials, err := a.relay.AuthorizeDevice(context.Background(), app.DeviceAuthorization{AccountID: accountID, DeviceName: deviceName})
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"accountId": credentials.AccountID, "deviceId": credentials.DeviceID}, nil
}
func (a *App) RelayURL() string                     { return a.relay.URL() }
func (a *App) CheckRelayHealth() error              { return a.relay.Health(context.Background()) }
func (a *App) GeneratePairingCode() (string, error) { return a.relay.PairingCode() }
func (a *App) ConfirmPairing(code string) (storage.PairingState, error) {
	return a.pairing.Confirm(context.Background(), code)
}
func (a *App) PairingState() (storage.PairingState, error) { return a.pairing.State() }
func (a *App) ClearPairing() error                         { return a.pairing.Clear(context.Background()) }
func (a *App) PublishTestAlert() error {
	return a.relay.Publish(context.Background(), protocol.IntelAlert{Title: "Test alert", Summary: "Desktop test alert", Severity: protocol.SeverityInfo, Confidence: 1})
}
func (a *App) FetchAlerts(cursor int64, limit int) (app.DataPage[app.AlertItem], error) {
	return a.relay.FetchAlerts(context.Background(), cursor, limit)
}
func (a *App) FetchOutbox(cursor int64, limit int) (app.DataPage[app.OutboxItem], error) {
	return a.relay.FetchOutbox(context.Background(), cursor, limit)
}
func (a *App) FetchEVEContractItems(characterID, contractID int64) ([]map[string]any, error) {
	return a.relay.FetchEVEContractItems(context.Background(), characterID, contractID)
}
func (a *App) FetchEVEContractBids(characterID, contractID int64) ([]map[string]any, error) {
	return a.relay.FetchEVEContractBids(context.Background(), characterID, contractID)
}
func (a *App) FetchEVEMailBody(characterID, mailID int64) (map[string]any, error) {
	return a.relay.FetchEVEMailBody(context.Background(), characterID, mailID)
}
func (a *App) FetchEVEKillmailDetail(characterID, killmailID int64, hash string) (map[string]any, error) {
	return a.relay.FetchEVEKillmailDetail(context.Background(), characterID, killmailID, hash)
}
func (a *App) FetchEVESyncStatus() ([]app.EVESyncJob, error) {
	return a.relay.FetchEVESyncStatus(context.Background())
}
func (a *App) FetchEVEAccountCharacters() ([]app.CharacterSnapshot, error) {
	return a.relay.FetchAccountCharacterSnapshots(context.Background())
}
func (a *App) FetchEVECharacterSnapshots(characterID int64) ([]app.CharacterSnapshot, error) {
	return a.relay.FetchCharacterSnapshots(context.Background(), characterID)
}
func (a *App) FetchEVECharacterSnapshot(characterID int64, domain string) (app.CharacterSnapshot, error) {
	return a.relay.FetchCharacterSnapshot(context.Background(), characterID, domain)
}
func (a *App) FetchEVEStatus() (app.EVEStatus, error) {
	return a.relay.FetchEVEStatus(context.Background())
}
func (a *App) FetchEVEMarketPrices() ([]app.EVEMarketPrice, error) {
	return a.relay.FetchEVEMarketPrices(context.Background())
}
func (a *App) FetchEVECorporation(corporationID int64) (app.EVECorporation, error) {
	return a.relay.FetchEVECorporation(context.Background(), corporationID)
}
func (a *App) FetchEVEAlliance(allianceID int64) (app.EVEAlliance, error) {
	return a.relay.FetchEVEAlliance(context.Background(), allianceID)
}
func (a *App) FetchEVEUniverseStation(stationID int64) (app.EVEStation, error) {
	return a.relay.FetchEVEUniverseStation(context.Background(), stationID)
}
func (a *App) FetchEVEUniverseNames(ids []int64) ([]app.EVEUniverseName, error) {
	return a.relay.FetchEVEUniverseNames(context.Background(), ids)
}
func (a *App) FetchEVEUniverseType(typeID int64) (app.EVEItemType, error) {
	return a.relay.FetchEVEUniverseType(context.Background(), typeID)
}
func (a *App) FetchEVEUniverseSystem(systemID int64) (app.EVESolarSystem, error) {
	return a.relay.FetchEVEUniverseSystem(context.Background(), systemID)
}
func (a *App) FetchEVESecureTradeRoute(fromSystemID, toSystemID int64, minSecurity float64, maxJumps int) (app.TradeRouteResult, error) {
	return a.relay.SecureTradeRoute(context.Background(), fromSystemID, toSystemID, minSecurity, maxJumps)
}
func (a *App) FetchEVESecureTradeRoutes(queries []app.TradeRouteQuery) ([]app.TradeRouteResult, error) {
	return a.relay.SecureTradeRoutes(context.Background(), queries)
}
func (a *App) FetchEVETradeRoute(fromSystemID, toSystemID int64, mode string, minSecurity float64) (evesde.RouteResult, error) {
	var floor *float64
	if minSecurity >= -1 {
		floor = &minSecurity
	}
	return a.sde.Route(context.Background(), fromSystemID, toSystemID, mode, floor)
}
func (a *App) FetchEVECharacterTradeContext(characterID int64, maximumISK, reserveISK float64) (app.CharacterTradeContext, error) {
	snaps, err := a.relay.FetchCharacterSnapshots(context.Background(), characterID)
	if err != nil {
		return app.CharacterTradeContext{}, err
	}
	max := maximumISK
	return app.BuildCharacterTradeContext(snaps, app.TradeBudget{MaximumISK: &max, ReserveISK: reserveISK}, tradeCargoResolver{a: a})
}

type tradeCargoResolver struct{ a *App }

func (r tradeCargoResolver) ShipCargoCapacity(typeID int64) (float64, string, bool, error) {
	t, err := r.a.relay.FetchEVEUniverseType(context.Background(), typeID)
	if err != nil {
		return 0, "", false, err
	}
	return t.Capacity, "ESI universe type base capacity", false, nil
}

func (a *App) SearchEVETradeCandidates(req app.TradeCandidateSearch) (app.TradeCandidatePage, error) {
	return a.relay.SearchTradeCandidates(context.Background(), req)
}
func (a *App) FetchEVETradeRegionCollection(regionID int64) (app.TradeRegionStatus, error) {
	return a.relay.TradeRegionCollection(context.Background(), regionID)
}
func (a *App) CollectEVETradeRegion(regionID int64) (app.TradeRegionStatus, error) {
	return a.relay.CollectTradeRegion(context.Background(), regionID)
}
func (a *App) ComposeEVETradePlan(req app.TradePackRequest) ([]map[string]any, error) {
	return a.relay.ComposeTradePlan(context.Background(), req)
}
func (a *App) ChainEVETradePlans(req app.TradeChainRequest) ([]map[string]any, error) {
	return a.relay.ChainTradePlans(context.Background(), req)
}
func (a *App) FetchEVETradeHubs() ([]app.EVETradeHub, error) {
	return a.relay.FetchEVETradeHubs(context.Background())
}
func (a *App) FetchEVEHubComparison(typeID int64) ([]app.EVEHubQuote, error) {
	return a.relay.FetchEVEHubComparison(context.Background(), typeID)
}
func (a *App) FetchEVETradePlans(req app.EVETradePlanRequest) ([]map[string]any, error) {
	return a.relay.FetchEVETradePlans(context.Background(), req)
}
func (a *App) FetchEVEMarketOrders(regionID int64, orderType string, typeID int64) ([]app.EVEMarketOrder, error) {
	return a.relay.FetchEVEMarketOrders(context.Background(), regionID, orderType, typeID)
}
func (a *App) FetchEVEMarketHistory(regionID, typeID int64) ([]app.EVEMarketHistory, error) {
	return a.relay.FetchEVEMarketHistory(context.Background(), regionID, typeID)
}
func (a *App) FetchConversations() ([]app.AgentConversation, error) {
	return a.relay.FetchConversations(context.Background())
}
func (a *App) StartRealtime(accountID, deviceID string) error {
	return a.realtime.Start(context.Background(), accountID, deviceID)
}
func (a *App) StopRealtime()                      { a.realtime.Stop() }
func (a *App) RealtimeStatus() app.RealtimeStatus { return a.realtime.Status() }
func (a *App) AckRealtimeEvent(event app.RealtimeEvent, status string) error {
	return a.realtime.Ack(context.Background(), event, status)
}
func (a *App) FetchRealtimeSnapshot() (string, error) {
	raw, err := a.relay.FetchRealtimeSnapshot(context.Background())
	return string(raw), err
}
func (a *App) NextRealtimeEvent() (app.RealtimeEvent, bool) {
	select {
	case event := <-a.realtime.Events():
		return event, true
	default:
		return app.RealtimeEvent{}, false
	}
}
func (a *App) StartAgentRun(conversationID, messageID string) (app.AgentRun, error) {
	return a.relay.StartRun(context.Background(), conversationID, messageID)
}
func (a *App) GetAgentRun(conversationID, runID string) (app.AgentRun, error) {
	return a.relay.GetRun(context.Background(), conversationID, runID)
}
func (a *App) ListAgentRunEvents(conversationID, runID string, after, limit int) ([]app.AgentRunEvent, int64, bool, error) {
	return a.relay.ListRunEvents(context.Background(), conversationID, runID, after, limit)
}
func (a *App) CancelAgentRun(conversationID, runID string) (app.AgentRun, error) {
	return a.relay.CancelRun(context.Background(), conversationID, runID)
}
func (a *App) FetchConversationMessages(conversationID string, cursor, limit int) (app.ConversationMessagePage, error) {
	return a.relay.FetchConversationMessages(context.Background(), conversationID, cursor, limit)
}

func (a *App) SendAgentMessage(conversationID, clientMessageID, operation, body string) (map[string]interface{}, error) {
	message, duplicate, err := a.relay.SendAgentMessage(context.Background(), conversationID, clientMessageID, operation, body)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"message": message, "duplicate": duplicate}, nil
}
func (a *App) ParseIntel(text string) []protocol.IntelFinding {
	return app.ParseIntel(text, "clipboard", time.Now().UTC())
}
func (a *App) Startup(ctx context.Context) {
	// Restore is best effort: first run has no pairing state, and a temporary
	// vault/database failure must not prevent the desktop UI from opening.
	if err := a.pairing.Restore(ctx); err != nil {
		a.startupErr = "恢复本地授权失败：" + err.Error()
	}
	// Official builds inject this non-secret URL at compile time. Keep source and
	// public examples redacted while making first-run production builds usable.
	if a.relay.URL() == "" {
		u := strings.TrimSpace(officialRelayURL)
		if u == "" {
			u = strings.TrimSpace(os.Getenv("ALICE_OFFICIAL_RELAY_URL"))
		}
		if u != "" {
			_ = a.relay.SetURL(u)
		}
	}
	if a.ingestion != nil {
		a.ingestion.Start(ctx)
	} else {
		_ = a.chatLogs.Start(ctx)
		_ = a.gameLogs.Start(ctx)
	}
}
func (a *App) ChatLogStatus() app.ChatLogStatus {
	if a.ingestion != nil {
		return a.ingestion.ChatStatus()
	}
	return a.chatLogs.Status()
}
func (a *App) RecentChatIntel() []app.ChatIntelEvent {
	if a.ingestion != nil {
		return a.ingestion.RecentChat()
	}
	return a.chatLogs.Recent()
}
func (a *App) GameLogStatus() app.GameLogStatus {
	if a.ingestion != nil {
		return a.ingestion.GameStatus()
	}
	return a.gameLogs.Status()
}
func (a *App) RecentGameEvents() []app.GameLogEvent {
	if a.ingestion != nil {
		return a.ingestion.RecentGame()
	}
	return a.gameLogs.Recent()
}
func (a *App) StoredLocalEvents(sourceKind string, limit int) ([]storage.NormalizedLocalEvent, error) {
	if a.ingestion == nil {
		return []storage.NormalizedLocalEvent{}, nil
	}
	return a.ingestion.StoredEvents(sourceKind, limit)
}
func (a *App) PublishChatIntel(eventID string) error {
	event, ok := a.chatLogs.Find(eventID)
	if !ok || len(event.Findings) == 0 {
		return errors.New("chat intel event not found")
	}
	severity := protocol.SeverityInfo
	for _, finding := range event.Findings {
		if finding.Stance == "hostile" {
			severity = protocol.SeverityWarning
			break
		}
	}
	values := make([]string, 0, len(event.Findings))
	for _, finding := range event.Findings {
		values = append(values, finding.Value)
	}
	return a.relay.Publish(context.Background(), protocol.IntelAlert{Title: "EVE Intel · " + event.Channel, Summary: event.Author + ": " + strings.Join(values, ", "), Severity: severity, Confidence: event.Findings[0].Confidence})
}
func (a *App) SetChatLogsDirectory(path string) error {
	if err := a.chatLogs.SetDirectory(path); err != nil {
		return err
	}
	return a.chatLogs.Start(context.Background())
}
func (a *App) RestartChatLogWatcher() error {
	a.chatLogs.Stop()
	return a.chatLogs.Start(context.Background())
}
func (a *App) Shutdown(_ context.Context) {
	a.once.Do(func() {
		if a.ingestion != nil {
			a.ingestion.Stop()
		}
		a.chatLogs.Stop()
		a.gameLogs.Stop()
		a.realtime.Stop()
		if a.close != nil {
			_ = a.close()
		}
	})
}
func (a *App) Health() string { return "ok" }

// SetSDEDirectory selects a local directory or imports a normalized JSONL file;
// no commands or URLs are executed. The historical API name is preserved.
func (a *App) SetSDEDirectory(path string) error {
	if strings.EqualFold(filepath.Ext(strings.TrimSpace(path)), ".jsonl") {
		_, err := a.sde.ImportNormalized(context.Background(), path)
		return err
	}
	return a.sde.SetDirectory(path)
}
func (a *App) SDEDirectory() string                            { return a.sde.Directory() }
func (a *App) ReindexSDE() (eve.SDEStatus, error)              { return a.sde.Reindex(context.Background()) }
func (a *App) SDEStatus() eve.SDEStatus                        { return a.sde.Status() }
func (a *App) SDETypeNames(ids []int64) map[int64]string       { return a.sde.TypeNames(ids) }
func (a *App) SDELocationNames(ids []int64) map[int64]string   { return a.sde.LocationNames(ids) }
func (a *App) QuerySDE(query string, limit int) []eve.SDEEntry { return a.sde.Query(query, limit) }

// QueryESI accepts an allow-listed ESI path, never an arbitrary URL.
func (a *App) QueryESI(path string) (eve.ESIResponse, error) {
	return a.esi.Get(context.Background(), path)
}

// OpenLocalAgentSession creates a short-lived bearer capability for the paired
// mobile protocol. The Agent and all desktop operations remain local.
func (a *App) OpenLocalAgentSession() (map[string]interface{}, error) {
	id, expires, err := a.localAgent.OpenSession()
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"sessionId": id, "expiresAt": expires}, nil
}

func (a *App) CloseLocalAgentSession(sessionID string) { a.localAgent.CloseSession(sessionID) }

// HandleLocalAgentRequest accepts a protocol JSON request and returns a JSON
// response. Unknown operations, including shell/command execution, are denied.
func (a *App) HandleLocalAgentRequest(request string) (string, error) {
	response, err := a.localAgent.HandleJSON(context.Background(), []byte(request))
	return string(response), err
}

func main() {
	appInstance, err := NewPersistentApp()
	if err != nil {
		panic(err)
	}
	if err := wails.Run(&options.App{AssetServer: &assetserver.Options{Assets: assets}, Title: "EVE 助手", Width: 1280, Height: 800, OnStartup: appInstance.Startup, OnShutdown: appInstance.Shutdown, Bind: []interface{}{appInstance}}); err != nil {
		panic(err)
	}
}
