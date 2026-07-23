package api

import (
	"log"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"
)

type ChatElement struct {
	LatestMessage string `json:"latest_message"`
	LatestTS      int64
	Sender        string
	Pinned        bool  `json:"pinned"`
	PinnedAt      int64 `json:"pinned_at"`
	Archived      bool  `json:"archived"`
	UnreadCount   int   `json:"unread_count"`
	MarkedUnread  bool  `json:"marked_unread"`
	Contact

	// Community linkage (populated for groups that belong to a community).
	ParentJID         string `json:"parent_jid,omitempty"`
	ParentName        string `json:"parent_name,omitempty"`
	IsCommunityGroup  bool   `json:"is_community_group"`
	IsCommunityParent bool   `json:"is_community_parent"`
	IsDefaultSubGroup bool   `json:"is_default_sub_group"`
}

// ToggleChatPin pins/unpins a chat: syncs the change to other devices via
// app state and records it locally for the chat list.
func (a *Api) ToggleChatPin(jidStr string, pinned bool) error {
	jid, err := types.ParseJID(jidStr)
	if err != nil {
		return err
	}
	// Local-first: the app's own state must not depend on app-state sync
	// health. Sync to other devices is best-effort (self-heals via
	// resyncAppState on the next connect if the hash chain is broken).
	if err := a.messageStore.SetChatPinned(jidStr, pinned, time.Now().Unix()); err != nil {
		return err
	}
	runtime.EventsEmit(a.ctx, "wa:chat_list_refresh")
	if err := a.waClient.SendAppState(a.ctx, appstate.BuildPin(jid, pinned)); err != nil {
		log.Println("ToggleChatPin: app state sync failed (kept local):", err)
		a.startBackground(a.resyncAppState)
	}
	return nil
}

// ToggleChatArchive archives/unarchives a chat: syncs to other devices via
// app state and records it locally for the chat list.
func (a *Api) ToggleChatArchive(jidStr string, archived bool) error {
	jid, err := types.ParseJID(jidStr)
	if err != nil {
		return err
	}
	// Local-first, same reasoning as ToggleChatPin.
	if err := a.messageStore.SetChatArchived(jidStr, archived, time.Now().Unix()); err != nil {
		return err
	}
	runtime.EventsEmit(a.ctx, "wa:chat_list_refresh")
	if err := a.waClient.SendAppState(a.ctx, appstate.BuildArchive(jid, archived, time.Time{}, nil)); err != nil {
		log.Println("ToggleChatArchive: app state sync failed (kept local):", err)
		a.startBackground(a.resyncAppState)
	}
	return nil
}

func (a *Api) GetChatList() ([]ChatElement, error) {
	cmList := a.messageStore.GetChatList()
	pinnedChats := a.messageStore.GetPinnedChats()
	archivedChats := a.messageStore.GetArchivedChats()
	unread := a.messageStore.GetAllUnread()
	ce := make([]ChatElement, len(cmList))
	for i, cm := range cmList {
		var fc Contact
		var parentJID, parentName string
		var isCommunityGroup, isCommunityParent, isDefaultSub bool

		if cm.JID.Server == types.GroupServer {
			// Local-only on purpose: this runs at startup before the client
			// has connected, so it must never touch the network. Empty names
			// are healed asynchronously by repairGroupNames after Connected.
			name := ""
			if groupInfo, err := a.cw.FetchGroup(cm.JID.String()); err == nil {
				name = groupInfo.Name
				parentJID = groupInfo.ParentJID
				parentName = groupInfo.ParentName
				if parentName == "" && parentJID != "" {
					parentName = a.cw.ParentCommunityName(parentJID)
				}
				isCommunityGroup = parentJID != ""
				isCommunityParent = groupInfo.IsParent
				isDefaultSub = groupInfo.IsDefaultSub
			} else {
				log.Println("GetChatList: group lookup failed, using fallback:", cm.JID.String(), err)
			}
			if name == "" {
				// A single unknown/left group must not blank the whole chat
				// list. Fall back to the JID so the chat still renders.
				name = cm.JID.User
			}
			fc = Contact{
				JID:      cm.JID.String(),
				FullName: name,
			}
		} else {
			contact, err := a.waClient.Store.Contacts.GetContact(a.ctx, cm.JID)
			if err != nil {
				// Same here: degrade to the JID rather than failing everything.
				log.Println("GetChatList: contact lookup failed, using fallback:", cm.JID.String(), err)
				fc = Contact{
					JID:      cm.JID.String(),
					PushName: cm.JID.User,
				}
			} else {
				fc = Contact{
					JID:        cm.JID.String(),
					Short:      contact.FirstName,
					FullName:   contact.FullName,
					PushName:   contact.PushName,
					IsBusiness: contact.BusinessName != "",
				}
			}
		}
		pinnedAt, pinned := pinnedChats[cm.JID.String()]
		_, archived := archivedChats[cm.JID.String()]
		u := unread[a.canonicalJID(cm.JID)]
		ce[i] = ChatElement{
			LatestMessage:     cm.MessageText,
			LatestTS:          cm.MessageTime,
			Sender:            cm.Sender,
			Pinned:            pinned,
			PinnedAt:          pinnedAt,
			Archived:          archived,
			UnreadCount:       u.Count,
			MarkedUnread:      u.MarkedUnread,
			Contact:           fc,
			ParentJID:         parentJID,
			ParentName:        parentName,
			IsCommunityGroup:  isCommunityGroup,
			IsCommunityParent: isCommunityParent,
			IsDefaultSubGroup: isDefaultSub,
		}
	}
	return ce, nil
}

// GetChannelList returns the followed Channels (newsletter feeds), named via
// their newsletter metadata.
func (a *Api) GetChannelList() ([]ChatElement, error) {
	cmList := a.messageStore.GetChannelList()
	ce := make([]ChatElement, len(cmList))
	for i, cm := range cmList {
		name := cm.JID.User
		if info, err := a.waClient.GetNewsletterInfo(a.ctx, cm.JID); err == nil && info != nil && info.ThreadMeta.Name.Text != "" {
			name = info.ThreadMeta.Name.Text
		}
		ce[i] = ChatElement{
			LatestMessage: cm.MessageText,
			LatestTS:      cm.MessageTime,
			Sender:        cm.Sender,
			Contact:       Contact{JID: cm.JID.String(), FullName: name},
		}
	}
	return ce, nil
}

func (a *Api) SendChatPresence(jid string, cp types.ChatPresence, cpm types.ChatPresenceMedia) error {
	parsedJid, err := types.ParseJID(jid)
	if err != nil {
		return err
	}
	return a.waClient.SendChatPresence(a.ctx, parsedJid, cp, cpm)
}
