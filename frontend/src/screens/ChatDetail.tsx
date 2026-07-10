import { useEffect, useState, useRef, useCallback, useMemo } from "react"
import {
  SendMessage,
  StopVoiceRecording,
  FetchMessagesPaged,
  GetPinnedMessages,
  FetchMessagesAround,
  SearchMessages,
  SendChatPresence,
  GetGroupInfo,
  GetProfile,
  MarkRead,
} from "../../wailsjs/go/api/Api"
import { store } from "../../wailsjs/go/models"
import { EventsOn } from "../../wailsjs/runtime/runtime"
import { useMessageStore, useUIStore, useChatStore } from "../store"
import { MessageList, type MessageListHandle } from "../components/chat/MessageList"
import { ChatHeader } from "../components/chat/ChatHeader"
import { ChatInput } from "../components/chat/ChatInput"
import { ChatInfo } from "../components/chat/ChatInfo"
import clsx from "clsx"
import { formatPhone } from "../lib/utils"
import gsap from "gsap"
import { useGSAP } from "@gsap/react"
import { getEase } from "../store/useEaseStore"

interface ChatDetailProps {
  chatId: string
  chatName: string
  chatAvatar?: string
  onBack?: () => void
  // When the chat is opened from a global content-search result, this seeds the
  // in-chat search so it jumps straight to the matching message.
  initialSearch?: string
}

const PAGE_SIZE = 50
// Virtuoso needs a stable, large starting index so it can decrement as older
// messages are prepended, keeping the scroll position anchored.
const START_INDEX = 1_000_000

function blobToDataURL(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(reader.result as string)
    reader.onerror = () => reject(reader.error)
    reader.readAsDataURL(blob)
  })
}

export function ChatDetail({ chatId, chatName, chatAvatar, onBack, initialSearch }: ChatDetailProps) {
  const {
    messages,
    setMessages,
    updateMessage,
    prependMessages,
    setActiveChatId,
    addPendingMessage,
    updatePendingMessageToSent,
    removeMessage,
  } = useMessageStore()
  const { setTypingIndicator, showEmojiPicker, setShowEmojiPicker, chatInfoOpen, setChatInfoOpen } =
    useUIStore()
  const { chatsById } = useChatStore()

  const chatMessages = messages[chatId] || []
  const [inputText, setInputText] = useState("")
  const [pastedImage, setPastedImage] = useState<string | null>(null)
  const [selectedFile, setSelectedFile] = useState<File | null>(null)
  const [selectedFileType, setSelectedFileType] = useState<string>("")
  const [replyingTo, setReplyingTo] = useState<store.DecodedMessage | null>(null)
  const [typingTimeout, setTypingTimeout] = useState<NodeJS.Timeout | null>(null)

  const [mentionableContacts, setMentionableContacts] = useState<any[]>([])
  const [selectedMentions, setSelectedMentions] = useState<any[]>([])
  const [hasMore, setHasMore] = useState(true)
  const [isLoadingMore, setIsLoadingMore] = useState(false)
  const [initialLoad, setInitialLoad] = useState(true)
  const [isReady, setIsReady] = useState(false)
  const [isAtBottom, setIsAtBottom] = useState(true)
  const [highlightedMessageId, setHighlightedMessageId] = useState<string | null>(null)
  const [firstItemIndex, setFirstItemIndex] = useState(START_INDEX)

  // In-chat search state.
  const [searchOpen, setSearchOpen] = useState(false)
  const [searchQuery, setSearchQuery] = useState("")
  const [searchResults, setSearchResults] = useState<store.DecodedMessage[]>([])
  const [searchIndex, setSearchIndex] = useState(0)
  // True after jumping into history (loaded set may not include the latest
  // messages), so the scroll-to-bottom button reloads the latest page.
  const [viewingHistory, setViewingHistory] = useState(false)
  const pendingJumpRef = useRef<string | null>(null)

  // Reset the Virtuoso anchor synchronously when switching chats so the fresh
  // list (see key={chatId} on <MessageList>) starts from a clean base.
  const anchorChatIdRef = useRef(chatId)
  if (anchorChatIdRef.current !== chatId) {
    anchorChatIdRef.current = chatId
    setFirstItemIndex(START_INDEX)
  }

  const messageListRef = useRef<MessageListHandle>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const emojiPickerRef = useRef<HTMLDivElement>(null)
  const emojiButtonRef = useRef<HTMLButtonElement>(null)
  const scrollButtonRef = useRef<HTMLButtonElement>(null)
  const sentMediaCache = useRef<Map<string, string>>(new Map())
  const isComposingRef = useRef(false)
  const requestGenerationRef = useRef(0)
  const loadingMoreRef = useRef(false)
  const hasMoreRef = useRef(true)
  const loadMorePromiseRef = useRef<Promise<store.DecodedMessage[]> | null>(null)
  const initialLoadPromiseRef = useRef<Promise<void> | null>(null)
  const highlightTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const easeShowRef = useRef(getEase("DropDown", "open"))
  const easeHideRef = useRef(getEase("DropDown", "close"))

  const currentChat = chatsById.get(chatId)
  const chatType = currentChat?.type || "contact"
  const [chatSubtitle, setChatSubtitle] = useState("")
  const [pinnedMessages, setPinnedMessages] = useState<store.PinnedMessage[]>([])
  // Which pinned message the banner jumps to next (cycles like WhatsApp).
  const pinnedCycleRef = useRef(0)

  const loadPinned = useCallback(async () => {
    try {
      const pins = await GetPinnedMessages(chatId)
      setPinnedMessages(pins || [])
    } catch (err) {
      console.error("Failed to load pinned messages:", err)
      setPinnedMessages([])
    }
  }, [chatId])

  useEffect(() => {
    pinnedCycleRef.current = 0
    loadPinned()
    const unsub = EventsOn("wa:pinned_update", (data: { chatId: string }) => {
      if (data?.chatId === chatId) loadPinned()
    })
    return unsub
  }, [chatId, loadPinned])

  const pinnedIds = useMemo(() => new Set(pinnedMessages.map(p => p.message_id)), [pinnedMessages])

  useEffect(() => {
    setChatSubtitle("")
    const loadMentionableContacts = async () => {
      if (chatType === "group") {
        try {
          const groupInfo = await GetGroupInfo(chatId)

          // WhatsApp-style participants line under the group name:
          // "You, Alice, ~ Bob, +91 98765 43210, …"
          const participantLabel = (c: any) =>
            c.full_name ||
            (c.push_name ? `~ ${c.push_name}` : "") ||
            c.short ||
            (c.phno ? formatPhone(c.phno) : "")
          try {
            const self = await GetProfile("")
            const others = groupInfo.group_participants
              .map((p: any) => p.contact)
              .filter((c: any) => c && c.phno !== self.phno && c.jid !== self.jid)
            setChatSubtitle(["You", ...others.map(participantLabel).filter(Boolean)].join(", "))
            setMentionableContacts(others)
          } catch (err) {
            const contacts = groupInfo.group_participants.map((p: any) => p.contact)
            setChatSubtitle(contacts.map(participantLabel).filter(Boolean).join(", "))
            setMentionableContacts(contacts)
          }
        } catch (error) {
          console.error("Failed to fetch group info:", error)
          setMentionableContacts([])
        }
      } else {
        setMentionableContacts([])
      }
    }
    loadMentionableContacts()
  }, [chatId, chatType])

  const scrollToBottom = useCallback((instant = false) => {
    requestAnimationFrame(() => {
      messageListRef.current?.scrollToBottom(instant ? "auto" : "smooth")
    })
  }, [])

  // Jump to a message that may or may not be in the currently loaded window.
  // If it isn't loaded, fetch a window centred on it (via FetchMessagesAround)
  // and scroll once it renders (see the pendingJumpRef effect below).
  const jumpToMessage = useCallback(
    async (messageId: string, keepHighlight = false) => {
      const loaded = useMessageStore.getState().messages[chatId] || []
      const inWindow = loaded.some((m: any) => m.Info.ID === messageId)
      if (inWindow) {
        messageListRef.current?.scrollToMessage(messageId)
        setHighlightedMessageId(messageId)
      } else {
        try {
          const window = await FetchMessagesAround(chatId, messageId, 25)
          if (window && window.length) {
            pendingJumpRef.current = messageId
            setViewingHistory(true)
            setFirstItemIndex(START_INDEX)
            setHasMore(true)
            setMessages(chatId, window)
            setHighlightedMessageId(messageId)
          }
        } catch (err) {
          console.error("Jump-to-message failed:", err)
        }
      }
      // For one-off jumps (quoted replies) fade the highlight; search keeps it
      // on the active match until the next navigation / close.
      if (!keepHighlight) {
        setTimeout(() => setHighlightedMessageId(null), 1200)
      }
    },
    [chatId, setMessages],
  )

  const handleQuotedClick = useCallback(
    (messageId: string) => {
      jumpToMessage(messageId, false)
    },
    [jumpToMessage],
  )

  // After a windowed jump, scroll to the target once it has rendered.
  useEffect(() => {
    const id = pendingJumpRef.current
    if (!id) return
    if (chatMessages.some((m: any) => m.Info.ID === id)) {
      requestAnimationFrame(() => messageListRef.current?.scrollToMessage(id))
      pendingJumpRef.current = null
    }
  }, [chatMessages])

  const handleAtBottomChange = useCallback((atBottom: boolean) => {
    setIsAtBottom(atBottom)
  }, [])

  // Focus the composer as soon as a chat is opened so the user can type
  // immediately (like WhatsApp).
  useEffect(() => {
    textareaRef.current?.focus()
  }, [chatId])

  // Navigate to a specific search result (keeps the match highlighted).
  const goToResult = useCallback(
    (idx: number) => {
      if (idx < 0 || idx >= searchResults.length) return
      setSearchIndex(idx)
      jumpToMessage(searchResults[idx].Info.ID, true)
    },
    [searchResults, jumpToMessage],
  )

  const closeSearch = useCallback(() => {
    setSearchOpen(false)
    setSearchQuery("")
    setSearchResults([])
    setSearchIndex(0)
    setHighlightedMessageId(null)
  }, [])

  // Debounced in-chat search: query the DB, then jump to the newest match.
  useEffect(() => {
    if (!searchOpen) return
    const q = searchQuery.trim()
    if (!q) {
      setSearchResults([])
      setSearchIndex(0)
      return
    }
    const t = setTimeout(async () => {
      try {
        const res = await SearchMessages(chatId, q, 200)
        const list = res || []
        setSearchResults(list)
        setSearchIndex(0)
        if (list.length) jumpToMessage(list[0].Info.ID, true)
      } catch (err) {
        console.error("Search failed:", err)
      }
    }, 250)
    return () => clearTimeout(t)
  }, [searchQuery, searchOpen, chatId, jumpToMessage])

  // ESC: close overlays first (info panel, emoji picker, reply), then leave
  // the chat back to the list.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return
      if (chatInfoOpen) {
        setChatInfoOpen(false)
        return
      }
      if (showEmojiPicker) {
        setShowEmojiPicker(false)
        return
      }
      if (replyingTo) {
        setReplyingTo(null)
        return
      }
      if (onBack) {
        onBack()
      } else {
        useChatStore.getState().selectChat(null)
      }
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [chatInfoOpen, showEmojiPicker, replyingTo, onBack, setChatInfoOpen, setShowEmojiPicker])

  const loadInitialMessages = useCallback(
    async (generation: number) => {
      setInitialLoad(true)
      setIsReady(false)
      const beforeRequest = new Map(
        (useMessageStore.getState().messages[chatId] || []).map(message => [
          message.Info?.ID,
          message,
        ]),
      )
      try {
        const msgs = await FetchMessagesPaged(chatId, PAGE_SIZE, 0, "")
        if (requestGenerationRef.current !== generation) return
        const loadedMsgs = msgs || []

        // Do not let the initial database snapshot overwrite an optimistic or
        // live message that arrived while the request was in flight.
        const current = useMessageStore.getState().messages[chatId] || []
        const currentByID = new Map(current.map(message => [message.Info?.ID, message]))
        const loadedIDs = new Set(loadedMsgs.map(message => message.Info?.ID))
        const merged = loadedMsgs.map(message => {
          const currentMessage = currentByID.get(message.Info?.ID)
          return currentMessage && currentMessage !== beforeRequest.get(message.Info?.ID)
            ? currentMessage
            : message
        })
        for (const message of current) {
          const id = message.Info?.ID
          if (id && !loadedIDs.has(id) && (!beforeRequest.has(id) || message.isPending)) {
            merged.push(message)
          }
        }
        setMessages(chatId, merged)
        const more = loadedMsgs.length >= PAGE_SIZE
        hasMoreRef.current = more
        setHasMore(more)

        requestAnimationFrame(() => {
          if (requestGenerationRef.current !== generation) return
          setIsReady(true)
          setInitialLoad(false)
        })
      } catch (err) {
        if (requestGenerationRef.current !== generation) return
        console.error("Initial load failed:", err)
        setInitialLoad(false)
      }
    },
    [chatId, setMessages],
  )

  const loadMoreMessages = useCallback((): Promise<store.DecodedMessage[]> => {
    if (loadMorePromiseRef.current) return loadMorePromiseRef.current
    if (!hasMoreRef.current || loadingMoreRef.current) return Promise.resolve([])

    const currentMessages = useMessageStore.getState().messages[chatId] || []
    if (currentMessages.length === 0) return Promise.resolve([])

    const generation = requestGenerationRef.current
    const oldestMessage = currentMessages[0]
    const beforeTimestamp = Math.floor(new Date(oldestMessage.Info.Timestamp).getTime() / 1000)
    loadingMoreRef.current = true
    setIsLoadingMore(true)

    const request = (async () => {
      try {
        const msgs =
          (await FetchMessagesPaged(chatId, PAGE_SIZE, beforeTimestamp, oldestMessage.Info.ID)) ||
          []
        if (requestGenerationRef.current !== generation) return []

        if (msgs.length > 0) {
          // Keep firstItemIndex and the prepended data in the same request
          // generation; a response from an abandoned chat cannot move this list.
          setFirstItemIndex(prev => prev - msgs.length)
          prependMessages(chatId, msgs)
        }
        const more = msgs.length >= PAGE_SIZE
        hasMoreRef.current = more
        setHasMore(more)
        return msgs
      } catch (err) {
        if (requestGenerationRef.current === generation) {
          console.error("Load more failed:", err)
        }
        return []
      }
    })()

    loadMorePromiseRef.current = request
    void request.finally(() => {
      if (loadMorePromiseRef.current === request) loadMorePromiseRef.current = null
      if (requestGenerationRef.current === generation) {
        loadingMoreRef.current = false
        setIsLoadingMore(false)
      }
    })
    return request
  }, [chatId, prependMessages])

  const handleQuotedClick = useCallback(
    async (messageId: string) => {
      const generation = requestGenerationRef.current
      if (initialLoadPromiseRef.current) await initialLoadPromiseRef.current
      if (requestGenerationRef.current !== generation) return
      let found = useMessageStore
        .getState()
        .messages[chatId]?.some(message => message.Info?.ID === messageId)

      // Pins and replies can point beyond the initial page. Load contiguous
      // older pages until the target is present or history is exhausted.
      while (!found && hasMoreRef.current && requestGenerationRef.current === generation) {
        const page = await loadMoreMessages()
        if (requestGenerationRef.current !== generation || page.length === 0) return
        found = page.some(message => message.Info?.ID === messageId)
      }
      if (!found || requestGenerationRef.current !== generation) return

      requestAnimationFrame(() => {
        requestAnimationFrame(() => {
          if (requestGenerationRef.current !== generation) return
          if (!messageListRef.current?.scrollToMessage(messageId)) return
          setHighlightedMessageId(messageId)
          if (highlightTimerRef.current) clearTimeout(highlightTimerRef.current)
          highlightTimerRef.current = setTimeout(() => {
            setHighlightedMessageId(null)
            highlightTimerRef.current = null
          }, 500)
        })
      })
    },
    [chatId, loadMoreMessages],
  )

  useEffect(() => {
    if (isAtBottom) {
      const messageIds = chatMessages.map((m: any) => m?.Info?.ID).filter((id: any) => !!id)
      if (messageIds.length > 0) {
        MarkRead(chatId, messageIds, "read-msg").catch(err => {
          console.error("Failed to mark messages as read:", err)
        })
      }
    }
  }, [isAtBottom, chatId, chatMessages])

  const handleInputChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const newValue = e.target.value
    setInputText(newValue)
    if (selectedMentions.length > 0) {
      setSelectedMentions(prev =>
        prev.filter(mention => {
          let name = mention.full_name
          if (!name) {
            if (mention.push_name) {
              name = `~ ${mention.push_name}`
            } else {
              name = mention.short || mention.phno
            }
          }
          return newValue.includes(`@${name}`)
        }),
      )
    }

    if (!isComposingRef.current) {
      isComposingRef.current = true
      setTypingIndicator(chatId, true)
      SendChatPresence(chatId, "composing", "").catch(() => {})
    }

    if (typingTimeout) clearTimeout(typingTimeout)
    const timeout = setTimeout(() => {
      isComposingRef.current = false
      SendChatPresence(chatId, "paused", "").catch(() => {})
      setTypingIndicator(chatId, false)
    }, 1500)
    setTypingTimeout(timeout)
  }

  const handleSendMessage = async () => {
    if (!inputText.trim() && !pastedImage && !selectedFile) return

    const textToSend = inputText
    const imageToSend = pastedImage
    const fileToSend = selectedFile
    const fileTypeToSend = selectedFileType
    const quotedMessageId = replyingTo?.Info.ID

    // Generate a temporary ID for optimistic update
    const tempId = `temp-${Date.now()}-${Math.random()}`

    // Create pending message
    const pendingMessage: any = {
      tempId,
      isPending: true,
      Info: {
        ID: tempId,
        IsFromMe: true,
        Timestamp: new Date().toISOString(),
        PushName: "You",
        Sender: "",
      },
      Content: {},
    }

    // Set content based on message type
    if (imageToSend) {
      pendingMessage.Content = {
        imageMessage: {
          caption: textToSend || "",
          mimetype: "image/png",
          _tempImage: imageToSend,
        },
      }
    } else if (fileToSend) {
      if (fileTypeToSend === "image") {
        pendingMessage.Content = {
          imageMessage: {
            caption: textToSend || "",
            mimetype: fileToSend.type,
            _tempFile: fileToSend,
          },
        }
      } else if (fileTypeToSend === "video") {
        pendingMessage.Content = {
          videoMessage: {
            caption: textToSend || "",
            mimetype: fileToSend.type,
            _tempFile: fileToSend,
          },
        }
      } else if (fileTypeToSend === "audio") {
        pendingMessage.Content = {
          audioMessage: {
            mimetype: fileToSend.type,
            _tempFile: fileToSend,
          },
        }
      } else {
        pendingMessage.Content = {
          documentMessage: {
            caption: textToSend || "",
            fileName: fileToSend.name,
            mimetype: fileToSend.type,
          },
        }
      }
    } else {
      pendingMessage.Content = {
        conversation: textToSend,
      }
    }

    // Add quoted message if replying
    if (quotedMessageId && replyingTo) {
      const contextInfo = {
        quotedMessage: replyingTo.Content,
        participant: replyingTo.Info.Sender,
        stanzaId: replyingTo.Info.ID,
      }

      if (pendingMessage.Content.conversation) {
        pendingMessage.Content = {
          extendedTextMessage: {
            text: pendingMessage.Content.conversation,
            contextInfo,
          },
        }
        delete pendingMessage.Content.conversation
      } else if (pendingMessage.Content.imageMessage) {
        pendingMessage.Content.imageMessage.contextInfo = contextInfo
      } else if (pendingMessage.Content.videoMessage) {
        pendingMessage.Content.videoMessage.contextInfo = contextInfo
      } else if (pendingMessage.Content.audioMessage) {
        pendingMessage.Content.audioMessage.contextInfo = contextInfo
      } else if (pendingMessage.Content.documentMessage) {
        pendingMessage.Content.documentMessage.contextInfo = contextInfo
      }
    }

    // Add pending message to store immediately
    addPendingMessage(chatId, pendingMessage)

    // Clear input
    setInputText("")
    setPastedImage(null)
    setSelectedFile(null)
    setReplyingTo(null)
    setSelectedMentions([])

    // Virtuoso follows appended messages when already at the bottom. When the
    // sender is reading older history, explicitly take them to their new post.
    if (!isAtBottom) scrollToBottom(false)

    let processedText = textToSend
    const mentionsToSend: string[] = []
    if (selectedMentions.length > 0) {
      const sortedMentions = [...selectedMentions].sort((a, b) => {
        let nameA = a.full_name
        if (!nameA) nameA = a.push_name ? `~ ${a.push_name}` : a.short || a.phno

        let nameB = b.full_name
        if (!nameB) nameB = b.push_name ? `~ ${b.push_name}` : b.short || b.phno

        return nameB.length - nameA.length
      })

      for (const mention of sortedMentions) {
        let name = mention.full_name
        if (!name) {
          if (mention.push_name) {
            name = `~ ${mention.push_name}`
          } else {
            name = mention.short || mention.phno
          }
        }
        const mentionText = `@${name}`

        if (processedText.includes(mentionText)) {
          const userPart = mention.jid.split("@")[0]
          const replacement = `@${userPart}`

          processedText = processedText.replaceAll(mentionText, replacement)

          mentionsToSend.push(mention.jid)
        }
      }
    }

    try {
      if (imageToSend) {
        const base64 = imageToSend.split(",")[1]
        const mimetype = imageToSend.match(/^data:([^;,]+)/)?.[1] || "image/png"
        await SendMessage(chatId, {
          type: "image",
          clientTempId: tempId,
          base64Data: base64,
          mimetype,
          text: processedText,
          quotedMessageId,
          mentions: mentionsToSend,
        })
      } else if (fileToSend) {
        const dataURL = await blobToDataURL(fileToSend)
        const base64 = dataURL.split(",")[1]
        await SendMessage(chatId, {
          type: fileTypeToSend,
          clientTempId: tempId,
          base64Data: base64,
          mimetype: fileToSend.type || "application/octet-stream",
          fileName: fileToSend.name,
          text: processedText,
          quotedMessageId,
          mentions: mentionsToSend,
        })
      } else {
        await SendMessage(chatId, {
          type: "text",
          clientTempId: tempId,
          text: processedText,
          quotedMessageId,
          mentions: mentionsToSend,
        })
      }
    } catch (err) {
      console.error("Failed to send:", err)
      // Optionally, mark message as failed or remove it
    }
  }

  const handleStopVoiceNote = async () => {
    try {
      // Backend records + sends and returns the audio so we can play our own
      // sent note locally (own sends aren't re-fetched from the server).
      const dataUrl = await StopVoiceRecording(chatId)
      const tempId = `temp-${Date.now()}-${Math.random()}`
      if (dataUrl) sentMediaCache.current.set(tempId, dataUrl)
      addPendingMessage(chatId, {
        tempId,
        isPending: false,
        Info: {
          ID: tempId,
          IsFromMe: true,
          Timestamp: new Date().toISOString(),
          PushName: "You",
          Sender: "",
        },
        Content: { audioMessage: { mimetype: "audio/ogg" } },
      })
      requestAnimationFrame(() => scrollToBottom(false))
    } catch (err) {
      console.error("Failed to send voice note:", err)
    }
  }

  useEffect(() => {
    const generation = ++requestGenerationRef.current
    loadingMoreRef.current = false
    loadMorePromiseRef.current = null
    hasMoreRef.current = true
    setFirstItemIndex(START_INDEX)
    setHasMore(true)
    setIsLoadingMore(false)
    setIsAtBottom(true)
    setActiveChatId(chatId)
    const initialRequest = loadInitialMessages(generation)
    initialLoadPromiseRef.current = initialRequest
    void initialRequest.finally(() => {
      if (initialLoadPromiseRef.current === initialRequest) initialLoadPromiseRef.current = null
    })
    return () => {
      if (requestGenerationRef.current === generation) requestGenerationRef.current++
      loadingMoreRef.current = false
      loadMorePromiseRef.current = null
      initialLoadPromiseRef.current = null
      if (highlightTimerRef.current) {
        clearTimeout(highlightTimerRef.current)
        highlightTimerRef.current = null
      }
    }
  }, [chatId, loadInitialMessages, setActiveChatId])

  // When the chat is opened from a global content-search result, open the
  // in-chat search seeded with the term so it jumps to the matching message.
  // Keyed on chatId so it only fires on (re)open, not on every keystroke.
  const initialSearchRef = useRef(initialSearch)
  initialSearchRef.current = initialSearch
  useEffect(() => {
    const seed = initialSearchRef.current
    if (seed) {
      setSearchOpen(true)
      setSearchQuery(seed)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [chatId])

  useEffect(() => {
    // New messages from events still use the old Message format for real-time updates
    // They will be compatible due to the Info and Content structure
    const unsub = EventsOn(
      "wa:new_message",
      (data: { chatId: string; message: any; clientTempId?: string }) => {
        if (data?.chatId === chatId && data.message?.Info?.ID) {
          // Use getState to avoid depending on messages array and causing re-subscriptions
          const currentMessages = useMessageStore.getState().messages[chatId] || []
          const hasPendingMessage = currentMessages.some((m: any) => m.isPending)

          if (hasPendingMessage && data.message.Info?.IsFromMe && data.clientTempId) {
            const pendingMessages = currentMessages.filter((m: any) => m.isPending)
            const pending = pendingMessages.find((m: any) => m.tempId === data.clientTempId)
            if (pending) {
              for (const body of ["imageMessage", "videoMessage", "audioMessage"]) {
                const transient =
                  pending.Content?.[body]?._tempImage || pending.Content?.[body]?._tempFile
                if (transient && data.message.Content?.[body]) {
                  data.message.Content[body][
                    transient instanceof File ? "_tempFile" : "_tempImage"
                  ] = transient
                }
              }
              updatePendingMessageToSent(data.chatId, pending.tempId, data.message)
            } else {
              updateMessage(data.chatId, data.message)
            }
          } else {
            updateMessage(data.chatId, data.message)
          }
        }
      },
    )

    return () => unsub()
  }, [chatId, updateMessage, updatePendingMessageToSent])

  // Remove messages deleted locally (delete-for-me), revoked by us, or revoked
  // remotely by another participant.
  useEffect(() => {
    const unsub = EventsOn("wa:message_deleted", (data: { chatId: string; messageId: string }) => {
      if (data?.chatId === chatId) removeMessage(chatId, data.messageId)
    })
    return () => unsub()
  }, [chatId, removeMessage])

  useGSAP(() => {
    if (!scrollButtonRef.current) return

    if (isAtBottom) {
      gsap.to(scrollButtonRef.current, {
        opacity: 0,
        duration: 0.3,
        ease: easeHideRef.current,
      })
    } else {
      gsap.to(scrollButtonRef.current, {
        opacity: 1,
        duration: 0.3,
        ease: easeShowRef.current,
      })
    }
  }, [isAtBottom])

  return (
    <div className="flex h-full min-w-0">
      <div className="flex flex-col flex-1 min-w-0">
        <ChatHeader
          chatName={chatName}
          chatSubtitle={chatSubtitle}
          chatAvatar={chatAvatar}
          onBack={onBack}
          onInfoClick={() => setChatInfoOpen(!chatInfoOpen)}
          onSearchClick={() => setSearchOpen(o => !o)}
        />

        {/* Pinned-messages banner: shows the latest pin, click cycles through
            pins newest-first and jumps to each message (WhatsApp behavior). */}
        {pinnedMessages.length > 0 && (
          <div
            onClick={() => {
              const idx = pinnedCycleRef.current % pinnedMessages.length
              const target = pinnedMessages[pinnedMessages.length - 1 - idx]
              pinnedCycleRef.current = idx + 1
              handleQuotedClick(target.message_id)
            }}
            className="flex items-center gap-2 border-b border-gray-200 bg-light-secondary px-4 py-2 text-sm cursor-pointer dark:border-white/5 dark:bg-dark-bg"
          >
            <svg
              viewBox="0 0 24 24"
              width="16"
              height="16"
              className="shrink-0 fill-current text-[#1b9a58] dark:text-[#21c063]"
            >
              <path d="M16 3a1 1 0 0 1 .95 1.31l-.9 2.72 3.42 3.42a1 1 0 0 1-.21 1.57l-3.62 2.07-1.9 4.75a1 1 0 0 1-1.64.33L9 16.07l-4.29 4.3-1.42-1.42 4.3-4.29-3.1-3.1a1 1 0 0 1 .33-1.64l4.75-1.9 2.07-3.62A1 1 0 0 1 12.5 4z" />
            </svg>
            <span
              className="flex-1 truncate text-gray-700 dark:text-gray-200 [&_*]:inline"
              dangerouslySetInnerHTML={{
                __html: pinnedMessages[pinnedMessages.length - 1].text || "Pinned message",
              }}
            />
            {pinnedMessages.length > 1 && (
              <span className="shrink-0 text-xs text-gray-500 dark:text-[#8696a0]">
                {pinnedMessages.length}
              </span>
            )}
          </div>
        )}

        {searchOpen && (
          <div className="flex items-center gap-2 p-2 bg-light-secondary dark:bg-dark-secondary border-b border-gray-200 dark:border-dark-tertiary">
            <div className="flex-1 flex items-center bg-light-tertiary dark:bg-dark-tertiary rounded-full px-3 py-1.5">
              <input
                autoFocus
                type="text"
                placeholder="Search in this chat"
                className="bg-transparent border-none outline-none text-sm w-full text-light-text dark:text-dark-text placeholder-gray-500"
                value={searchQuery}
                onChange={e => setSearchQuery(e.target.value)}
                onKeyDown={e => {
                  if (e.key === "Escape") closeSearch()
                  else if (e.key === "Enter") {
                    e.preventDefault()
                    // Enter -> older match, Shift+Enter -> newer match.
                    goToResult(e.shiftKey ? searchIndex - 1 : searchIndex + 1)
                  }
                }}
              />
            </div>
            <span className="text-xs text-gray-500 dark:text-gray-400 min-w-14 text-center tabular-nums">
              {searchQuery.trim()
                ? searchResults.length
                  ? `${searchIndex + 1}/${searchResults.length}`
                  : "0/0"
                : ""}
            </span>
            <button
              onClick={() => goToResult(searchIndex - 1)}
              disabled={searchIndex <= 0}
              className="p-1.5 rounded-full text-gray-500 dark:text-gray-400 hover:bg-hover-icons disabled:opacity-30"
              title="Newer match"
            >
              <svg viewBox="0 0 24 24" width="18" height="18" className="fill-current">
                <path d="M7.41 15.41L12 10.83l4.59 4.58L18 14l-6-6-6 6z" />
              </svg>
            </button>
            <button
              onClick={() => goToResult(searchIndex + 1)}
              disabled={searchIndex >= searchResults.length - 1}
              className="p-1.5 rounded-full text-gray-500 dark:text-gray-400 hover:bg-hover-icons disabled:opacity-30"
              title="Older match"
            >
              <svg viewBox="0 0 24 24" width="18" height="18" className="fill-current">
                <path d="M7.41 8.59L12 13.17l4.59-4.58L18 10l-6 6-6-6z" />
              </svg>
            </button>
            <button
              onClick={closeSearch}
              className="p-1.5 rounded-full text-gray-500 dark:text-gray-400 hover:bg-hover-icons"
              title="Close search (Esc)"
            >
              <svg viewBox="0 0 24 24" width="18" height="18" className="fill-current">
                <path d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z" />
              </svg>
            </button>
          </div>
        )}

        <div className="flex-1 relative overflow-hidden">
          {/* Static chat wallpaper: painted once behind the list instead of
              scrolling (and repainting) with it — big scroll-perf win. */}
          <div className="chat-wallpaper absolute inset-0 pointer-events-none z-0" />
          {(initialLoad || !isReady) && (
            <div className="absolute inset-0 flex items-center justify-center bg-[#efeae2] dark:bg-[#0a0a0a] z-50">
              <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-green-500" />
            </div>
          )}

          <button
            ref={scrollButtonRef}
            onClick={() => {
              // After a search jump we may be showing a history window; reload
              // the latest page instead of just scrolling within it.
              if (viewingHistory) {
                setViewingHistory(false)
                loadInitialMessages(++requestGenerationRef.current)
              } else {
                scrollToBottom(false)
              }
            }}
            className="absolute bottom-4 right-8 bg-white dark:bg-received-bubble-dark-bg p-2 rounded-full shadow-lg border border-gray-200 dark:border-gray-700 z-100 hover:bg-gray-100 dark:hover:bg-[#2a3942]"
          >
            <svg
              viewBox="0 0 24 24"
              width="24"
              height="24"
              className="fill-current text-gray-600 dark:text-gray-400"
            >
              <path d="M12 16.17L4.83 9L3.41 10.41L12 19L20.59 10.41L19.17 9L12 16.17Z" />
            </svg>
          </button>

          <div className={clsx("relative z-10 h-full", (!isReady || initialLoad) && "invisible")}>
            <MessageList
              key={`${chatId}:${isReady ? "ready" : "loading"}`}
              ref={messageListRef}
              chatId={chatId}
              messages={chatMessages}
              firstItemIndex={firstItemIndex}
              sentMediaCache={sentMediaCache}
              onReply={setReplyingTo}
              onQuotedClick={handleQuotedClick}
              onLoadMore={() => void loadMoreMessages()}
              onAtBottomChange={handleAtBottomChange}
              pinnedIds={pinnedIds}
              isLoading={isLoadingMore}
              hasMore={isReady && hasMore}
              highlightedMessageId={highlightedMessageId}
            />
          </div>
        </div>
        <ChatInput
          chatId={chatId}
          inputText={inputText}
          pastedImage={pastedImage}
          selectedFile={selectedFile}
          selectedFileType={selectedFileType}
          showEmojiPicker={showEmojiPicker}
          textareaRef={textareaRef}
          fileInputRef={fileInputRef}
          emojiPickerRef={emojiPickerRef}
          emojiButtonRef={emojiButtonRef}
          replyingTo={replyingTo}
          mentionableContacts={mentionableContacts}
          onInputChange={handleInputChange}
          onKeyDown={e =>
            e.key === "Enter" && !e.shiftKey && (e.preventDefault(), handleSendMessage())
          }
          onPaste={async e => {
            // Chromium exposes pasted images synchronously via DataTransfer.
            const items = e.clipboardData?.items
            for (const item of items || []) {
              if (item.type.indexOf("image") !== -1) {
                const file = item.getAsFile()
                if (file) {
                  e.preventDefault()
                  setPastedImage(await blobToDataURL(file))
                  return
                }
              }
            }

            // WebKitGTK (Wails on Linux) does not put system-clipboard images
            // into DataTransfer, so fall back to the async Clipboard API.
            if (!navigator.clipboard?.read) return
            try {
              const clipboardItems = await navigator.clipboard.read()
              for (const clipboardItem of clipboardItems) {
                const imageType = clipboardItem.types.find(t => t.startsWith("image/"))
                if (imageType) {
                  const blob = await clipboardItem.getType(imageType)
                  setPastedImage(await blobToDataURL(blob))
                  return
                }
              }
            } catch (err) {
              console.error("Clipboard image read failed:", err)
            }
          }}
          onSendMessage={handleSendMessage}
          onFileSelect={e => {
            const file = e.target.files?.[0]
            if (file) {
              setSelectedFile(file)
              const generalType = file.type.split("/")[0]
              setSelectedFileType(
                generalType === "image" || generalType === "video" || generalType === "audio"
                  ? generalType
                  : "document",
              )
            }
          }}
          onRemoveFile={() => {
            setSelectedFile(null)
            setPastedImage(null)
          }}
          onEmojiClick={emoji => {
            setInputText(prev => prev + emoji)
            setShowEmojiPicker(false)
          }}
          onToggleEmojiPicker={() => setShowEmojiPicker(!showEmojiPicker)}
          onCancelReply={() => setReplyingTo(null)}
          onMentionAdd={contact => setSelectedMentions(prev => [...prev, contact])}
          selectedMentions={selectedMentions}
          onStopVoiceNote={handleStopVoiceNote}
        />
      </div>

      <ChatInfo
        chatId={chatId}
        chatName={chatName}
        chatType={chatType}
        chatAvatar={chatAvatar}
        isOpen={chatInfoOpen}
        onClose={() => setChatInfoOpen(false)}
      />
    </div>
  )
}
