package main

import (
	"embed"
	"log"

	"mqttc/internal/mqtt"
	"mqttc/internal/services"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	profiles := services.NewProfileService(services.KeyringSecrets{})
	sessions := services.NewSessionService(profiles, mqtt.Paho5Dialer{})

	app := application.New(application.Options{
		Name:        "mqttc",
		Description: "A small, fast MQTT client",
		Services: []application.Service{
			application.NewService(profiles),
			application.NewService(sessions),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:    "mqttc",
		Width:    1280,
		Height:   820,
		MinWidth: 900, MinHeight: 560,
		BackgroundColour: application.NewRGB(24, 24, 27),
		Mac: application.MacWindow{
			TitleBar:                application.MacTitleBarHiddenInset,
			InvisibleTitleBarHeight: 40,
			Backdrop:                application.MacBackdropTranslucent,
		},
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
