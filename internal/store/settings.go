package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/lugvitc/whats4linux/internal/misc"
)

// notificationsKey is the app_settings.json key for the global
// notifications on/off switch.
const notificationsKey = "notifications_enabled"

// notificationsEnabled caches the global notification switch so the hot
// notify path can read it without touching the settings map (which is not
// safe for concurrent access).
var notificationsEnabled atomic.Bool

func init() {
	// Default to enabled until LoadSettings reads the persisted value.
	notificationsEnabled.Store(true)
}

type settings struct {
	mu   sync.Mutex
	f    *os.File
	data map[string]any
}

var settingsInstance = &settings{
	data: make(map[string]any),
}

func LoadSettings() {
	var err error
	settingsInstance.f, err = os.OpenFile(filepath.Join(misc.ConfigDir, "app_settings.json"), os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		panic(err)
	}

	decoder := json.NewDecoder(settingsInstance.f)
	_ = decoder.Decode(&settingsInstance.data)
	notificationsEnabled.Store(notificationsEnabledFrom(settingsInstance.data))
}

func GetSettings() map[string]any {
	return settingsInstance.data
}

func SaveSettings(data map[string]any) error {
	settingsInstance.mu.Lock()
	defer settingsInstance.mu.Unlock()

	settingsInstance.data = data
	// Keep the cached switch in sync in case the whole settings map was
	// replaced by the frontend.
	notificationsEnabled.Store(notificationsEnabledFrom(data))

	return settingsInstance.writeLocked()
}

// GetNotificationsEnabled reports the global notification switch. Safe for
// concurrent use; defaults to true when the setting was never persisted.
func GetNotificationsEnabled() bool {
	return notificationsEnabled.Load()
}

// SetNotificationsEnabled persists the global notification switch to
// app_settings.json and updates the cached value.
func SetNotificationsEnabled(enabled bool) error {
	notificationsEnabled.Store(enabled)

	settingsInstance.mu.Lock()
	defer settingsInstance.mu.Unlock()

	// Copy-on-write so concurrent GetSettings readers never observe a map
	// mutation mid-flight.
	newData := make(map[string]any, len(settingsInstance.data)+1)
	for k, v := range settingsInstance.data {
		newData[k] = v
	}
	newData[notificationsKey] = enabled
	settingsInstance.data = newData

	return settingsInstance.writeLocked()
}

// notificationsEnabledFrom extracts the switch from a settings map, defaulting
// to true when unset or of an unexpected type.
func notificationsEnabledFrom(data map[string]any) bool {
	if v, ok := data[notificationsKey]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return true
}

// writeLocked persists the current settings map to disk. The caller must hold
// settingsInstance.mu.
func (s *settings) writeLocked() error {
	// Truncate the file before writing
	err := s.f.Truncate(0)
	if err != nil {
		return err
	}

	// Reset the file offset to the beginning
	_, err = s.f.Seek(0, 0)
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(s.f)
	return encoder.Encode(s.data)
}

func CloseSettings() error {
	return settingsInstance.f.Close()
}
