# EVE Assistant Mobile

Flutter mobile companion scaffold for the EVE Assistant desktop app.

## Current scaffold

- Login placeholder
- Device pairing placeholder
- Alert list placeholder
- Versioned protocol model (`MessageEnvelope`, `AlertEvent`)
- Relay API client interface and no-op implementation
- Planned transport: HTTPS REST + WSS, FCM/APNs

## Directory

```text
lib/
├── main.dart
├── api/api_client.dart
└── models/protocol.dart
```

Flutter was not available in the development environment, so this is a standard hand-written Flutter project skeleton. Run `flutter pub get` and `flutter run` after installing Flutter.

The mobile app should communicate with the public Go relay server, never directly expose or receive desktop EVE SSO tokens.
