# mqttc

A small, fast, open source desktop MQTT client. Connect to a broker, watch topics stream in real time, inspect payloads, and publish messages from a single window.

Built with Go and [Wails v3](https://v3.wails.io/) on the backend and Svelte 5 with Tailwind on the frontend. MQTT connectivity uses the Eclipse Paho v5 client.

## Features

- **Connection profiles** saved to your user config directory. Passwords go to the OS keychain and are never written to disk.
- **Multiple sessions** open at once, one per profile.
- **Topic tree** that fills in as messages arrive, with per-topic history.
- **Payload inspector** with JSON rendering and syntax highlighting.
- **Sparkline** of any numeric field in a topic's recent messages.
- **Publish bar** for sending messages to any topic.
- **TLS support**, with an insecure toggle for local dev brokers.

## Download

Grab the installer for your platform from the [download page](https://flyzard.github.io/mqtt-client/) or the [latest release](https://github.com/flyzard/mqtt-client/releases/latest):

- macOS: `mqttc-macos-universal.dmg` (Apple Silicon and Intel, macOS 13 or newer)
- Windows: `mqttc-windows-amd64-installer.exe`
- Linux: `mqttc-linux-amd64.deb`, `mqttc-linux-amd64.rpm`, or `mqttc-linux-amd64.AppImage` (needs GTK 4 and WebKitGTK 6.0, so Ubuntu 24.04, Debian 13, Fedora 40 or newer)

Every release ships a `SHA256SUMS.txt` you can check downloads against.

### First launch

The builds are not code signed yet, so each OS warns once.

- **macOS** reports the app as damaged because it was downloaded from the internet. Drag it to Applications, then run this once in Terminal:

  ```
  xattr -cr /Applications/mqttc.app
  ```

- **Windows** shows a SmartScreen dialog. Click "More info", then "Run anyway".
- **Linux** has no such gate. The AppImage needs to be made executable with `chmod +x` first.

## Development

Requires Go 1.27+, Node.js, and the [Wails v3 CLI](https://v3.wails.io/getting-started/installation/).

```
task dev      # run with hot reload
task build    # production build into bin/
task package  # platform installer / bundle
task check    # vet, tests, type-check
```

## Layout

- `main.go`: app entry point and service registration
- `internal/mqtt/`: broker sessions, message batching, and event delivery
- `internal/services/`: profiles, secrets, and the session API exposed to the UI
- `frontend/src/`: Svelte UI, with components under `components/` and state and helpers under `lib/`

## Contributing

Issues and pull requests are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for the checks to run and how contributions are licensed. [DESIGN.md](DESIGN.md) describes the visual system if you are touching the UI.

## License

mqttc is open source under the [MIT License](LICENSE). Third-party components and their licences are listed in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
