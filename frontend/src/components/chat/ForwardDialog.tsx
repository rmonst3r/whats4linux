import { useState } from "react"
import { createPortal } from "react-dom"
import { useChatStore } from "../../store"
import { ForwardMessage } from "../../../wailsjs/go/api/Api"
import { GroupIcon, UserAvatar } from "../../assets/svgs/chat_icons"

interface ForwardDialogProps {
  fromChatId: string
  messageId: string
  onClose: () => void
}

// A lightweight chat picker for forwarding a message to another chat.
export function ForwardDialog({ fromChatId, messageId, onClose }: ForwardDialogProps) {
  const chatIds = useChatStore(s => s.chatIds)
  const chatsById = useChatStore(s => s.chatsById)
  const [query, setQuery] = useState("")
  const [sendingTo, setSendingTo] = useState<string | null>(null)
  const [done, setDone] = useState<string | null>(null)

  const q = query.trim().toLowerCase()
  const filtered = chatIds.filter(id => {
    const c = chatsById.get(id)
    return c && (!q || c.name.toLowerCase().includes(q))
  })

  const forward = async (toId: string) => {
    if (sendingTo) return
    setSendingTo(toId)
    try {
      await ForwardMessage(fromChatId, messageId, toId)
      setDone(toId)
      setTimeout(onClose, 600)
    } catch (e) {
      console.error("Forward failed:", e)
      setSendingTo(null)
    }
  }

  return createPortal(
    <div
      className="fixed inset-0 z-[10000] flex items-center justify-center bg-black/40"
      onClick={onClose}
    >
      <div
        className="flex h-[70vh] w-96 flex-col overflow-hidden rounded-2xl bg-white shadow-xl dark:bg-dark-secondary"
        onClick={e => e.stopPropagation()}
      >
        <div className="p-4 pb-2">
          <div className="mb-3 flex items-center justify-between">
            <h3 className="text-base font-semibold text-gray-800 dark:text-gray-100">
              Forward to…
            </h3>
            <button
              onClick={onClose}
              className="text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
              aria-label="Close"
            >
              <svg viewBox="0 0 24 24" width="20" height="20" className="fill-current">
                <path d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z" />
              </svg>
            </button>
          </div>
          <input
            autoFocus
            type="text"
            placeholder="Search chats"
            value={query}
            onChange={e => setQuery(e.target.value)}
            onKeyDown={e => e.key === "Escape" && onClose()}
            className="w-full rounded-full bg-light-tertiary px-4 py-2 text-sm outline-none dark:bg-dark-tertiary dark:text-dark-text"
          />
        </div>

        <div className="flex-1 overflow-y-auto px-2 pb-2">
          {filtered.length === 0 ? (
            <p className="p-4 text-center text-sm text-gray-500">No chats found</p>
          ) : (
            filtered.map(id => {
              const c = chatsById.get(id)!
              return (
                <button
                  key={id}
                  onClick={() => forward(id)}
                  disabled={!!sendingTo}
                  className="flex w-full items-center gap-3 rounded-xl p-2 text-left hover:bg-gray-100 disabled:opacity-60 dark:hover:bg-dark-tertiary"
                >
                  <div className="flex h-10 w-10 shrink-0 items-center justify-center overflow-hidden rounded-full bg-gray-300 dark:bg-gray-600">
                    {c.avatar ? (
                      <img src={c.avatar} alt={c.name} className="h-full w-full object-cover" />
                    ) : c.type === "group" ? (
                      <GroupIcon />
                    ) : (
                      <UserAvatar />
                    )}
                  </div>
                  <span className="flex-1 truncate text-sm text-gray-800 dark:text-gray-100">
                    {c.name}
                  </span>
                  {done === id ? (
                    <span className="text-xs font-medium text-green-600 dark:text-green-400">
                      Sent
                    </span>
                  ) : sendingTo === id ? (
                    <span className="h-4 w-4 animate-spin rounded-full border-b-2 border-green-500" />
                  ) : null}
                </button>
              )
            })
          )}
        </div>
      </div>
    </div>,
    document.body,
  )
}
