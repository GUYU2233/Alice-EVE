package main

import (
	"context"
	"eve-assistant/desktop-app/internal/app"
	"eve-assistant/desktop-app/internal/eve"
	"eve-assistant/desktop-app/internal/protocol"
	"eve-assistant/desktop-app/internal/storage"
	"os"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

type App struct {
	service    *app.ApplicationService
	relay      *app.RelayClient
	pairing    *app.PairingManager
	store      storage.Store
	localAgent *app.LocalAgent
	sde        *eve.SDEIndex
	esi        *eve.ESIClient
	realtime   *app.RealtimeClient
	close      func() error
	once       sync.Once
}

// NewApp keeps the historical no-argument constructor for tests and callers
// that explicitly provide dependencies through NewAppWithStorage.
func NewApp() *App { return NewAppWithStorage(nil, nil) }

func NewAppWithStorage(store storage.Store, secrets storage.SecretStore) *App {
	relay := app.NewRelayClientWithSecrets(secrets)
	workspaceRoot, _ := os.Getwd()
	controller := app.NewLocalDesktopController("desktop", workspaceRoot)
	return &App{service: app.NewApplicationService(), relay: relay, pairing: app.NewPairingManager(store, relay), store: store, localAgent: app.NewLocalAgent(controller, nil), sde: eve.NewSDEIndex(""), esi: eve.NewESIClient(), realtime: app.NewRealtimeClient(relay, store)}
}

func NewPersistentApp() (*App, error) {
	store, secrets, closeDB, err := newPersistentDependencies()
	if err != nil {
		return nil, err
	}
	a := NewAppWithStorage(store, secrets)
	a.close = closeDB
	return a, nil
}

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
	_ = a.pairing.Restore(ctx)
}
func (a *App) Shutdown(_ context.Context) {
	a.once.Do(func() {
		a.realtime.Stop()
		if a.close != nil {
			_ = a.close()
		}
	})
}
func (a *App) Health() string { return "ok" }

// SetSDEDirectory selects a local directory only; no commands or URLs are executed.
func (a *App) SetSDEDirectory(path string) error               { return a.sde.SetDirectory(path) }
func (a *App) SDEDirectory() string                            { return a.sde.Directory() }
func (a *App) ReindexSDE() (eve.SDEStatus, error)              { return a.sde.Reindex(context.Background()) }
func (a *App) SDEStatus() eve.SDEStatus                        { return a.sde.Status() }
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
