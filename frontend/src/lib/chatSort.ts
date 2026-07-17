import type { ChatItem } from "../store/types"

/**
 * WhatsApp chat-list order: pinned chats first (most recent activity first
 * within the pinned block), then everything else by last-message time.
 */
export function sortChatItems(chats: ChatItem[]): ChatItem[] {
  return [...chats].sort((a, b) => {
    if (!!a.pinned !== !!b.pinned) return a.pinned ? -1 : 1
    return (b.timestamp || 0) - (a.timestamp || 0)
  })
}
