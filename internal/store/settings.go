package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/lugvitc/whats4linux/internal/misc"
)

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
}

func GetSettings() map[string]any {
	return settingsInstance.data
}

func SaveSettings(data map[string]any) error {
	settingsInstance.mu.Lock()
	defer settingsInstance.mu.Unlock()

	if settingsInstance.f == nil {
		return errors.New("settings store is not initialised")
	}

	settingsInstance.data = data

	// Truncate the file before writing
	err := settingsInstance.f.Truncate(0)
	if err != nil {
		return err
	}

	// Reset the file offset to the beginning
	_, err = settingsInstance.f.Seek(0, 0)
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(settingsInstance.f)
	err = encoder.Encode(settingsInstance.data)
	if err != nil {
		return err
	}

	return nil
}

func CloseSettings() error {
	settingsInstance.mu.Lock()
	defer settingsInstance.mu.Unlock()

	if settingsInstance.f == nil {
		return nil
	}

	err := settingsInstance.f.Close()
	settingsInstance.f = nil
	return err
}
