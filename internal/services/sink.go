package services

import (
	"mqttc/internal/mqtt"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Register typed events at init so Emit is type-checked and the binding
// generator emits typed event helpers for the frontend.
func init() {
	application.RegisterEvent[mqtt.StateChanged](mqtt.EvStateChanged)
	application.RegisterEvent[mqtt.MessagesBatch](mqtt.EvMessagesBatch)
	application.RegisterEvent[mqtt.TopicsChanged](mqtt.EvTopicsChanged)
	application.RegisterEvent[mqtt.SubscriptionsChanged](mqtt.EvSubsChanged)
}

type wailsSink struct{ app *application.App }

func (s wailsSink) OnState(e mqtt.StateChanged)     { s.app.Event.Emit(mqtt.EvStateChanged, e) }
func (s wailsSink) OnMessages(e mqtt.MessagesBatch) { s.app.Event.Emit(mqtt.EvMessagesBatch, e) }
func (s wailsSink) OnTopics(e mqtt.TopicsChanged)   { s.app.Event.Emit(mqtt.EvTopicsChanged, e) }
func (s wailsSink) OnSubscriptions(e mqtt.SubscriptionsChanged) {
	s.app.Event.Emit(mqtt.EvSubsChanged, e)
}
