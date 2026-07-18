import { useEffect, useState } from "react"
import clsx from "clsx"
import { FetchMessagesPaged, GetCachedImage, DownloadMedia } from "../../../wailsjs/go/api/Api"
import { store } from "../../../wailsjs/go/models"
import { useContactStore } from "../../store/useContactStore"

const STATUS_JID = "status@broadcast"
const STORY_MS = 5000

export interface StatusGroup {
  sender: string
  name: string
  items: store.DecodedMessage[]
}

function last<T>(a: T[]): T {
  return a[a.length - 1]
}

// List of people who have recent status updates. Clicking one opens the viewer.
export function StatusList({ onOpen }: { onOpen: (g: StatusGroup) => void }) {
  const [groups, setGroups] = useState<StatusGroup[]>([])
  const [loading, setLoading] = useState(true)
  const getContactName = useContactStore(s => s.getContactName)

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      const msgs = ((await FetchMessagesPaged(STATUS_JID, 300, 0, "")) ||
        []) as store.DecodedMessage[]
      const bySender = new Map<string, store.DecodedMessage[]>()
      for (const m of msgs) {
        const s = m.Info?.Sender
        if (!s) continue
        if (!bySender.has(s)) bySender.set(s, [])
        bySender.get(s)!.push(m)
      }
      const result: StatusGroup[] = []
      for (const [sender, items] of bySender) {
        items.sort((a, b) => a.Info.Timestamp.localeCompare(b.Info.Timestamp))
        let name = sender.split("@")[0].split(":")[0]
        try {
          name = (await getContactName(sender)) || name
        } catch {
          /* keep number */
        }
        result.push({ sender, name, items })
      }
      result.sort((a, b) =>
        last(b.items).Info.Timestamp.localeCompare(last(a.items).Info.Timestamp),
      )
      if (!cancelled) {
        setGroups(result)
        setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [getContactName])

  if (loading) return <div className="p-6 text-center text-sm text-gray-500">Loading status…</div>
  if (groups.length === 0)
    return <div className="p-6 text-center text-sm text-gray-500">No recent status updates</div>

  return (
    <div>
      {groups.map(g => (
        <button
          key={g.sender}
          onClick={() => onOpen(g)}
          className="flex w-full items-center gap-3 p-3 text-left hover:bg-gray-100 dark:hover:bg-dark-tertiary"
        >
          <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-full border-2 border-green-500 text-lg font-medium text-green-600 dark:text-green-400">
            {g.name.charAt(0).toUpperCase()}
          </div>
          <div className="min-w-0 flex-1">
            <div className="truncate font-medium">{g.name}</div>
            <div className="text-xs text-gray-500">
              {g.items.length} update{g.items.length > 1 ? "s" : ""}
            </div>
          </div>
        </button>
      ))}
    </div>
  )
}

// Full-screen story viewer: progress bars, auto-advance, tap/keyboard nav.
export function StoryViewer({ group, onClose }: { group: StatusGroup; onClose: () => void }) {
  const [idx, setIdx] = useState(0)
  const [mediaSrc, setMediaSrc] = useState<string | null>(null)

  const item = group.items[idx]
  const content = (item?.Content || {}) as any
  const kind: "image" | "video" | "text" = content.imageMessage
    ? "image"
    : content.videoMessage
      ? "video"
      : "text"
  const caption =
    content.conversation ||
    content.extendedTextMessage?.text ||
    content.imageMessage?.caption ||
    content.videoMessage?.caption ||
    ""

  const next = () => setIdx(i => (i < group.items.length - 1 ? i + 1 : (onClose(), i)))
  const prev = () => setIdx(i => (i > 0 ? i - 1 : i))

  // Load media for the current item.
  useEffect(() => {
    setMediaSrc(null)
    if (!item || kind === "text") return
    let cancelled = false
    ;(async () => {
      let src = ""
      if (kind === "image") src = (await GetCachedImage(item.Info.ID).catch(() => "")) || ""
      if (!src) src = (await DownloadMedia(STATUS_JID, item.Info.ID).catch(() => "")) || ""
      if (!cancelled) setMediaSrc(src || null)
    })()
    return () => {
      cancelled = true
    }
  }, [item, kind])

  // Auto-advance (videos advance on their own end).
  useEffect(() => {
    if (kind === "video") return
    const t = setTimeout(next, STORY_MS)
    return () => clearTimeout(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [idx, kind])

  // Keyboard nav.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose()
      else if (e.key === "ArrowRight") next()
      else if (e.key === "ArrowLeft") prev()
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  })

  return (
    <div className="fixed inset-0 z-[9999] flex flex-col bg-black">
      <div className="flex gap-1 p-2">
        {group.items.map((_, i) => (
          <div key={i} className="h-0.5 flex-1 overflow-hidden rounded bg-white/30">
            {i < idx ? (
              <div className="h-full w-full bg-white" />
            ) : i === idx && kind !== "video" ? (
              <div
                key={idx}
                className="story-active-bar h-full bg-white"
                style={{ animationDuration: `${STORY_MS}ms` }}
              />
            ) : i === idx ? (
              <div className="h-full w-full bg-white/70" />
            ) : null}
          </div>
        ))}
      </div>

      <div className="flex items-center justify-between px-4 pb-2 text-white">
        <div className="font-medium">{group.name}</div>
        <button
          onClick={onClose}
          className="text-2xl leading-none hover:text-white/70"
          title="Close (Esc)"
        >
          ×
        </button>
      </div>

      <div className="relative flex flex-1 items-center justify-center overflow-hidden">
        <button
          onClick={prev}
          className="absolute left-0 top-0 z-10 h-full w-1/3"
          aria-label="Previous"
        />
        <button
          onClick={next}
          className="absolute right-0 top-0 z-10 h-full w-1/3"
          aria-label="Next"
        />

        {kind === "text" && (
          <div
            className="max-w-lg px-8 text-center text-2xl leading-relaxed text-white [&_a]:underline"
            dangerouslySetInnerHTML={{ __html: caption }}
          />
        )}
        {kind === "image" &&
          (mediaSrc ? (
            <img src={mediaSrc} alt="" className="max-h-full max-w-full object-contain" />
          ) : (
            <Spinner />
          ))}
        {kind === "video" &&
          (mediaSrc ? (
            <video
              src={mediaSrc}
              autoPlay
              onEnded={next}
              className="max-h-full max-w-full object-contain"
            />
          ) : (
            <Spinner />
          ))}

        {kind !== "text" && caption && (
          <div
            className="pointer-events-none absolute bottom-6 left-0 right-0 px-8 text-center text-white [&_a]:underline"
            dangerouslySetInnerHTML={{ __html: caption }}
          />
        )}
      </div>
    </div>
  )
}

function Spinner() {
  return <div className="h-8 w-8 animate-spin rounded-full border-b-2 border-white" />
}
