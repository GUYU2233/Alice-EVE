# Pages

## `/` — Desktop workbench
Entry: `desktop-app/frontend/src/main.tsx`
Dependencies:
- `desktop-app/frontend/src/style.css`
- Wails runtime `window.go.main.App` methods: `RelayURL`, `SetRelayURL`, `CheckRelayHealth`, `GeneratePairingCode`, `PublishTestAlert`, `ParseIntel`

Rendered regions: header, connection and pairing controls, Intel textarea/parser, findings list, test alert card, outbox placeholder, workbench status card, and a settings alternate view.
