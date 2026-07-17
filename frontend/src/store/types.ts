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
