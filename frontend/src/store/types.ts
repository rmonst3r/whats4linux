export interface ChatItem {
  id: string
  name: string
  subtitle: string
  type: "group" | "contact"
  timestamp?: number
  avatar?: string
  unreadCount?: number
  sender?: string
  pinned?: boolean
  archived?: boolean
  /** Parent community JID when this chat is a community subgroup. */
  communityJid?: string
  /** Parent community display name (shown above the group name). */
  communityName?: string
  /** Parent community avatar for the stacked logo. */
  communityAvatar?: string
  isCommunityGroup?: boolean
  isCommunityParent?: boolean
}

export interface Message {
  id: string
  chatId: string
  content: any
  timestamp: number
  isFromMe: boolean
  sender?: string
  pushName?: string
}

export interface TypingIndicator {
  chatId: string
  isTyping: boolean
  userId?: string
}
