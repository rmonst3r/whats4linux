package api

import (
	"log"
	"time"

	"github.com/lugvitc/whats4linux/internal/store"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"
)

// Systray socket protocol for the notifications toggle.
const (
	trayCmdToggleNotifications = "toggle_notifications"
	trayCmdGetNotifState       = "get_notifications_state"
	trayNotifStateOn           = "notifications_state:on"
	trayNotifStateOff          = "notifications_state:off"
)

// notificationsStateMessage encodes the global switch for the systray.
func notificationsStateMessage(enabled bool) string {
	if enabled {
		return trayNotifStateOn
	}
	return trayNotifStateOff
}

// trayCommandHandler handles app-specific commands arriving from the systray
// over the unix socket. Replies keep the tray checkbox in sync.
func (a *Api) trayCommandHandler(cmd string) (string, bool) {
	switch cmd {
	case trayCmdToggleNotifications:
		// SetNotificationsEnabled pushes the new state to the tray itself,
		// so no extra reply is needed here.
		_ = a.SetNotificationsEnabled(!store.GetNotificationsEnabled())
		return "", true
	case trayCmdGetNotifState:
		return notificationsStateMessage(store.GetNotificationsEnabled()), true
	}
	return "", false
}

// GetNotificationsEnabled reports the global desktop notification switch.
func (a *Api) GetNotificationsEnabled() bool {
	return store.GetNotificationsEnabled()
}

// SetNotificationsEnabled flips the global desktop notification switch,
// persists it to app_settings.json, notifies the frontend and keeps the
// systray checkbox in sync.
func (a *Api) SetNotificationsEnabled(enabled bool) error {
	if err := store.SetNotificationsEnabled(enabled); err != nil {
		return err
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "wa:notifications_toggled", enabled)
	}
	// Best-effort push; the tray may not be connected.
	_ = a.us.SendCommand(notificationsStateMessage(enabled))
	return nil
}

// ToggleChatMute mutes (forever) or unmutes a chat. The mutation is sent via
// app state so it syncs to the phone, then persisted locally.
func (a *Api) ToggleChatMute(chatJID string, muted bool) error {
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return err
	}
	// Store mutes under the canonical (PN) JID so lookups from the message
	// path always hit the same key.
	jid = canonicalUserJID(a.ctx, a.waClient, jid)
	// Duration 0 with muted=true means "muted forever" (see appstate.BuildMute).
	if err := a.waClient.SendAppState(a.ctx, appstate.BuildMute(jid, muted, 0)); err != nil {
		return err
	}
	var mutedUntil int64 // 0 deletes the local mute row
	if muted {
		mutedUntil = -1 // forever
	}
	if err := a.messageStore.SetChatMuted(jid.String(), mutedUntil); err != nil {
		return err
	}
	runtime.EventsEmit(a.ctx, "wa:chat_mute_update", map[string]any{
		"chatId": jid.String(),
		"muted":  muted,
	})
	return nil
}

// IsChatMuted reports whether a chat is currently muted (locally persisted
// state, including mutes synced from the phone).
func (a *Api) IsChatMuted(chatJID string) (bool, error) {
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return false, err
	}
	jid = canonicalUserJID(a.ctx, a.waClient, jid)
	return a.messageStore.IsChatMuted(jid.String()), nil
}

// mutedUntilFromMuteEnd converts a whatsmeow mute end timestamp into the
// muted_until representation used by the store: -1 = muted forever, unix
// seconds = muted until then, 0 = not muted.
func mutedUntilFromMuteEnd(muted bool, end int64) int64 {
	if !muted {
		return 0
	}
	if end <= 0 {
		// whatsmeow uses -1 (or unset) for "muted forever".
		return -1
	}
	// App state mute end timestamps are unix milliseconds (see
	// appstate.BuildMute). Guard against second-based values just in case:
	// anything above ~year 5138 in seconds must be milliseconds.
	if end > 1e11 {
		return end / 1000
	}
	return end
}

// handleMuteEvent persists a mute change synced from another device (or from
// an app-state full sync) and notifies the frontend.
func (a *Api) handleMuteEvent(jid types.JID, muted bool, muteEnd int64) {
	mutedUntil := mutedUntilFromMuteEnd(muted, muteEnd)
	chatID := canonicalUserJID(a.ctx, a.waClient, jid).String()
	if err := a.messageStore.SetChatMuted(chatID, mutedUntil); err != nil {
		log.Println("Failed to persist chat mute state:", err)
		return
	}
	runtime.EventsEmit(a.ctx, "wa:chat_mute_update", map[string]any{
		"chatId": chatID,
		"muted":  mutedUntil == -1 || mutedUntil > time.Now().Unix(),
	})
}
