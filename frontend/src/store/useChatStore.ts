import { create } from "zustand"
import { useShallow } from "zustand/react/shallow"
import { useCallback } from "react"
import type { ChatItem } from "./types"
import { sortChatItems } from "../lib/chatSort"

interface ChatStore {
  // Use a Map for O(1) lookups and granular updates
  chatsById: Map<string, ChatItem>
  // Keep ordered list of chat IDs for rendering order
  chatIds: string[]
  selectedChatId: string | null
  selectedChatName: string
  selectedChatAvatar?: string
  selectedChatSender?: string
  searchTerm: string
  // Actions
  setChats: (chats: ChatItem[]) => void
  selectChat: (chat: ChatItem | null) => void
  setSearchTerm: (term: string) => void
  updateChatLastMessage: (
    chatId: string,
    message: string,
    timestamp?: number,
    sender?: string,
  ) => void
  updateSingleChat: (chatId: string, updates: Partial<ChatItem>) => void
  resortChats: () => void
  setUnread: (chatId: string, count: number, markedUnread: boolean) => void
  clearUnreadCount: (chatId: string) => void
  getChat: (chatId: string) => ChatItem | undefined
}

export const useChatStore = create<ChatStore>((set, get) => ({
  chatsById: new Map(),
  chatIds: [],
  selectedChatId: null,
  selectedChatName: "",
  selectedChatAvatar: undefined,
  selectedChatSender: undefined,
  searchTerm: "",

  setChats: chats =>
    set(state => {
      const newChatsById = new Map<string, ChatItem>()

      for (const chat of chats) {
        // The backend is authoritative for unread state (persisted in
        // messages.db and seeded from the server), so the rebuilt items already
        // carry the correct count — no need to preserve the previous value.
        newChatsById.set(chat.id, chat)
      }

      const newChatIds = sortChatItems([...newChatsById.values()]).map(c => c.id)
      return { chatsById: newChatsById, chatIds: newChatIds }
    }),

  selectChat: chat =>
    set({
      selectedChatId: chat?.id || null,
      selectedChatName: chat?.name || "",
      selectedChatAvatar: chat?.avatar,
      selectedChatSender: chat?.sender,
    }),

  setSearchTerm: term => set({ searchTerm: term }),

  // Recompute display order (pinned first, then recency) after an in-place
  // update like an optimistic pin toggle.
  resortChats: () =>
    set(state => ({
      chatIds: sortChatItems([...state.chatsById.values()]).map(c => c.id),
    })),

  // Update only a single chat without replacing the entire Map
  updateSingleChat: (chatId, updates) =>
    set(state => {
      const existingChat = state.chatsById.get(chatId)
      if (!existingChat) return state

      const newChatsById = new Map(state.chatsById)
      newChatsById.set(chatId, { ...existingChat, ...updates })

      return { chatsById: newChatsById }
    }),

  updateChatLastMessage: (chatId, message, timestamp, sender) =>
    set(state => {
      const existingChat = state.chatsById.get(chatId)
      if (!existingChat) return state

      const newChatsById = new Map(state.chatsById)
      newChatsById.set(chatId, {
        ...existingChat,
        subtitle: message,
        timestamp: timestamp || Date.now(),
        sender: sender !== undefined ? sender : existingChat.sender,
      })

      // Re-sort: keeps pinned chats above even when another chat gets a
      // new message.
      const newChatIds = sortChatItems([...newChatsById.values()]).map(c => c.id)

      return { chatsById: newChatsById, chatIds: newChatIds }
    }),

  // Apply an absolute unread state pushed by the backend (wa:unread_update).
  // Absolute rather than incremental so every device agrees on the badge.
  setUnread: (chatId, count, markedUnread) =>
    set(state => {
      const existingChat = state.chatsById.get(chatId)
      if (!existingChat) return state
      if (existingChat.unreadCount === count && !!existingChat.markedUnread === markedUnread) {
        return state
      }

      const newChatsById = new Map(state.chatsById)
      newChatsById.set(chatId, { ...existingChat, unreadCount: count, markedUnread })

      return { chatsById: newChatsById }
    }),

  clearUnreadCount: chatId =>
    set(state => {
      const existingChat = state.chatsById.get(chatId)
      if (!existingChat) return state

      const newChatsById = new Map(state.chatsById)
      newChatsById.set(chatId, { ...existingChat, unreadCount: 0, markedUnread: false })

      return { chatsById: newChatsById }
    }),

  getChat: chatId => get().chatsById.get(chatId),
}))

// Selector hook to get a single chat by ID - only re-renders when that specific chat changes
export const useChatById = (chatId: string) => {
  return useChatStore(useCallback((state: ChatStore) => state.chatsById.get(chatId), [chatId]))
}

// Selector hook to get only chat IDs (for list rendering) - doesn't re-render on chat content changes
export const useChatIds = () => {
  return useChatStore(useShallow((state: ChatStore) => state.chatIds))
}

// Selector for filtered chat IDs based on search and archive view.
export const useFilteredChatIds = (showArchived = false) => {
  return useChatStore(
    useShallow((state: ChatStore) => {
      const { chatIds, chatsById, searchTerm } = state
      const term = searchTerm.toLowerCase()

      return chatIds.filter(id => {
        const chat = chatsById.get(id)
        if (!chat) return false
        if (!!chat.archived !== showArchived) return false
        return !term || chat.name.toLowerCase().includes(term)
      })
    }),
  )
}

// Number of archived chats, for the "Archived" entry row.
export const useArchivedCount = () => {
  return useChatStore((state: ChatStore) => {
    let n = 0
    for (const chat of state.chatsById.values()) if (chat.archived) n++
    return n
  })
}

// Legacy helper to get chats as array (for backward compatibility)
export const useChatsArray = () => {
  return useChatStore(
    useShallow((state: ChatStore) => {
      return state.chatIds.map(id => state.chatsById.get(id)!).filter(Boolean)
    }),
  )
}
