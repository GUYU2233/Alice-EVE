package main

import (
	"context"
	"eve-assistant/desktop-app/internal/app"
	"eve-assistant/desktop-app/internal/protocol"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

type App struct{ service *app.ApplicationService; relay *app.RelayClient }

func NewApp() *App { return &App{service: app.NewApplicationService(), relay: app.NewRelayClient()} }
func (a *App) SetRelayURL(url string) error { return a.relay.SetURL(url) }
func (a *App) RelayURL() string { return a.relay.URL() }
func (a *App) CheckRelayHealth() error { return a.relay.Health(context.Background()) }
func (a *App) GeneratePairingCode() (string,error) { return a.relay.PairingCode() }
func (a *App) PublishTestAlert() error { return a.relay.Publish(context.Background(), protocol.IntelAlert{Title:"Test alert", Summary:"Desktop test alert", Severity:protocol.SeverityInfo, Confidence:1}) }
func (a *App) Startup(_ context.Context)  {}
func (a *App) Shutdown(_ context.Context) {}
func (a *App) Health() string             { return "ok" }

func main() {
	appInstance := NewApp()
	if err := wails.Run(&options.App{AssetServer: &assetserver.Options{Assets: assets}, Title: "EVE 助手", Width: 1280, Height: 800, OnStartup: appInstance.Startup, OnShutdown: appInstance.Shutdown, Bind: []interface{}{appInstance}}); err != nil {
		panic(err)
	}
}
