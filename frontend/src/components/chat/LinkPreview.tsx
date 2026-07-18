import { useEffect, useRef, useState } from "react"
import { GetLinkPreviewImage } from "../../../wailsjs/go/api/Api"
import { BrowserOpenURL } from "../../../wailsjs/runtime/runtime"
import { LRUCache } from "../../lib/lruCache"

export interface LinkPreviewData {
  url: string
  title: string
  description: string
  has_poster?: boolean
  thumbnail?: string
}

const posterCache = new LRUCache<string, string>(48, 24 * 1024 * 1024, value => value.length)
const posterRequests = new Map<string, Promise<string>>()

function loadPosterOnce(messageId: string) {
  const existing = posterRequests.get(messageId)
  if (existing) return existing
  const request = GetLinkPreviewImage(messageId).finally(() => {
    if (posterRequests.get(messageId) === request) posterRequests.delete(messageId)
  })
  posterRequests.set(messageId, request)
  return request
}

// Renders the WhatsApp link-preview card under a message, if one was stored.
export function LinkPreview({
  messageId,
  preview,
}: {
  messageId: string
  preview?: LinkPreviewData | null
}) {
  const [poster, setPoster] = useState<string>(() => posterCache.get(messageId) ?? "")
  const posterAreaRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    if (!preview?.has_poster || preview.thumbnail || posterCache.has(messageId)) return
    const area = posterAreaRef.current
    if (!area) return
    let cancelled = false
    let timer: ReturnType<typeof setTimeout> | undefined
    let started = false
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (!entry.isIntersecting || started) {
          if (timer && !started) clearTimeout(timer)
          return
        }
        timer = setTimeout(() => {
          started = true
          loadPosterOnce(messageId)
            .then(url => {
              if (!url) return
              posterCache.set(messageId, url)
              if (!cancelled) setPoster(url)
            })
            .catch(() => {})
        }, 200)
      },
      { rootMargin: "150px", threshold: 0.01 },
    )
    observer.observe(area)
    return () => {
      cancelled = true
      observer.disconnect()
      if (timer) clearTimeout(timer)
    }
  }, [messageId, preview?.has_poster, preview?.thumbnail])

  if (!preview) return null

  const image = preview.thumbnail || poster
  const hasImageArea = !!(preview.thumbnail || preview.has_poster)

  let domain = ""
  try {
    domain = new URL(preview.url).hostname.replace(/^www\./, "")
  } catch {
    /* ignore malformed URL */
  }

  return (
    <div
      onClick={() => preview.url && BrowserOpenURL(preview.url)}
      className="mt-1 max-w-72 overflow-hidden rounded-lg bg-black/10 dark:bg-black/25 cursor-pointer"
    >
      {/* Fixed height: an unconstrained image resizes the card (and the row)
          when it decodes, which shakes the virtualized list. */}
      {hasImageArea && (
        <div ref={posterAreaRef} className="w-full h-40 bg-black/20">
          {image && <img src={image} alt="" className="w-full h-full object-cover" />}
        </div>
      )}
      <div className="p-2">
        {preview.title && (
          <div className="text-sm font-medium line-clamp-2 leading-snug">{preview.title}</div>
        )}
        {preview.description && (
          <div className="mt-0.5 text-xs opacity-70 line-clamp-2">{preview.description}</div>
        )}
        {domain && (
          <div className="mt-1 text-[10px] uppercase tracking-wide opacity-50">{domain}</div>
        )}
      </div>
    </div>
  )
}
