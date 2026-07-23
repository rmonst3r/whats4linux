package api

import (
	"html"
	"strings"
	"time"

	"github.com/lugvitc/whats4linux/internal/misc"
	"github.com/lugvitc/whats4linux/internal/settings"
	"github.com/lugvitc/whats4linux/internal/store"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"go.mau.fi/whatsmeow/types"
)

func (a *Api) GetJIDUser(jid types.JID) string {
	return jid.User
}

func (a *Api) GetCustomCSS() string {
	return settings.GetCustomCSS()
}

func (a *Api) SetCustomCSS(css string) error {
	return settings.SetCustomCSS(css)
}

func (a *Api) GetCustomJS() string {
	return settings.GetCustomJS()
}

func (a *Api) SetCustomJS(js string) error {
	return settings.SetCustomJS(js)
}

func (a *Api) Reinitialize() error {
	return a.cw.Initialise(a.waClient)
}

// Logout unlinks this device from the WhatsApp account: the server removes it
// from Linked Devices and whatsmeow wipes the local session. On success the
// app quits (after a short delay so the frontend can show a goodbye note);
// the next launch starts fresh at the QR screen for re-linking. If the server
// request fails nothing is deleted, so the session stays usable.
func (a *Api) Logout() error {
	if err := a.waClient.Logout(a.ctx); err != nil {
		return err
	}
	go func() {
		time.Sleep(1500 * time.Millisecond)
		runtime.Quit(a.ctx)
	}()
	return nil
}

func (a *Api) SaveSettings(s map[string]any) error {
	return store.SaveSettings(s)
}

func (a *Api) GetSettings() map[string]any {
	return store.GetSettings()
}

func replaceMentions(text string, mentionedJIDs []string, a *Api) string {
	result := text

	for _, jid := range mentionedJIDs {
		parsedJID, err := types.ParseJID(jid)
		if err != nil {
			continue
		}
		parsedJID = canonicalUserJID(a.ctx, a.waClient, parsedJID)
		contact, _ := a.waClient.Store.Contacts.GetContact(a.ctx, parsedJID)
		displayName := contact.FullName
		if displayName == "" {
			displayName = "~ " + contact.PushName
		}
		if displayName == "" {
			displayName = parsedJID.User
		}

		mentionPattern := "@" + strings.Split(jid, "@")[0]
		mentionHTML := `<span class="mention">@` + html.EscapeString(displayName) + `</span>`
		result = strings.ReplaceAll(result, mentionPattern, mentionHTML)
	}

	return result
}

func (a *Api) GetProfileColor(jidStr string) string {
	return misc.GetProfileColor(jidStr)
}
