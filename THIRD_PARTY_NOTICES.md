# Third-party notices

mqttc is released under the MIT License (see `LICENSE`). The full dependency
list is in `go.mod` and `frontend/package.json`; each package's licence text
ships alongside it in the Go module cache and in `frontend/node_modules`, and
is available at the links below. Apart from the two exceptions here, every
dependency is under a permissive licence (MIT, Apache-2.0 or BSD Zero Clause)
that needs no further notice.

## Eclipse Paho Go client (EPL-2.0)

MQTT connectivity uses `github.com/eclipse/paho.golang`, unmodified, under
the Eclipse Public License 2.0. As the EPL requires, its source is available
at https://github.com/eclipse/paho.golang and the licence at
https://www.eclipse.org/legal/epl-2.0/. The Paho source is not distributed
with mqttc binaries.

## Fonts (OFL-1.1)

The application bundles two typefaces under the SIL Open Font License 1.1.
The OFL permits bundling and redistribution with software, but the fonts
may not be sold on their own, and the licence text must accompany them.

| Font | Source |
| --- | --- |
| Figtree | https://github.com/erikdkennedy/figtree |
| JetBrains Mono | https://github.com/JetBrains/JetBrainsMono |

Both are packaged via Fontsource (`@fontsource-variable/figtree`,
`@fontsource-variable/jetbrains-mono`), whose packages carry the licence
files.
