# mqttc

A small, fast desktop MQTT client. Connect to a broker, watch topics stream in real time, inspect payloads, and publish messages from a single window.

Built with Go and [Wails v3](https://v3.wails.io/) on the backend and Svelte 5 with Tailwind on the frontend. MQTT connectivity uses the Eclipse Paho v5 client.

## Features

- **Connection profiles** saved to your user config directory. Passwords go to the OS keychain and are never written to disk.
- **Multiple sessions** open at once, one per profile.
- **Topic tree** that fills in as messages arrive, with per-topic history.
- **Payload inspector** with JSON rendering and syntax highlighting.
- **Sparkline** of any numeric field in a topic's recent messages.
- **Publish bar** for sending messages to any topic.
- **TLS support**, with an insecure toggle for local dev brokers.

## Development

Requires Go 1.27+, Node.js, and the [Wails v3 CLI](https://v3.wails.io/getting-started/installation/).

```
task dev      # run with hot reload
task build    # production build into bin/
task package  # platform installer / bundle
```

Backend tests:

```
go test ./...
```

Frontend checks and tests:

```
cd frontend
npm run check
npm test
```

## Layout

- `main.go`: app entry point and service registration
- `internal/mqtt/`: broker sessions, message batching, and event delivery
- `internal/services/`: profiles, secrets, and the session API exposed to the UI
- `frontend/src/`: Svelte UI, with components under `components/` and state and helpers under `lib/`
