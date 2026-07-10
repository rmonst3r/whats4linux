import { useState, useEffect, useRef } from "react"
import { store } from "../../../wailsjs/go/models"
import { GetCachedImage, DownloadMedia, GetVideoThumbnail } from "../../../wailsjs/go/api/Api"
import { useUIStore } from "../../store"
import { LRUCache } from "../../lib/lruCache"

// TODO: fix word wrap for longer words in content

// GIFs (WhatsApp sends them as short muted videos) loop a few times then stop.
const MAX_GIF_LOOPS = 3

// Rendered bounds for chat images/GIF videos. These must match the Tailwind
// classes on the media elements below: min-w-75 (300px), max-w-82.5 (330px),
// max-h-100 (400px).
const MEDIA_MIN_W = 300
const MEDIA_MAX_W = 330
const MEDIA_MAX_H = 400

// Data URLs can be large. Keep enough recently viewed media to avoid flicker
// when Virtuoso remounts nearby rows without retaining every chat image for the
// lifetime of the process.
const imagePathCache = new LRUCache<string, string>(48, 32 * 1024 * 1024, value => value.length)
const videoThumbCache = new LRUCache<string, string>(100, 16 * 1024 * 1024, value => value.length)
const mediaRequests = new Map<string, Promise<string>>()

function loadMediaOnce(key: string, loader: () => Promise<string>): Promise<string> {
  const existing = mediaRequests.get(key)
  if (existing) return existing
  const request = loader().finally(() => {
    if (mediaRequests.get(key) === request) mediaRequests.delete(key)
  })
  mediaRequests.set(key, request)
  return request
}

// Fallback box for GIF videos whose dimensions were never stored (rows synced
// before dimension extraction existed). A fixed square with object-cover is
// deterministic: the placeholder and the loaded video occupy the same box, so
// the row height never changes.
const GIF_FALLBACK_BOX = { width: 256, height: 256 }
const IMAGE_FALLBACK_BOX = { width: 300, height: 256 }

// Computes the exact box CSS gives the loaded media from its intrinsic
// dimensions (stored by the backend in message_media), so the placeholder can
// reserve identical space up front. Without this the row height changes when
// the pixels arrive, which makes Virtuoso re-anchor and the list visibly
// jump while scrolling.
export function mediaBox(w?: number, h?: number): { width: number; height: number } | null {
  if (!w || !h || w <= 0 || h <= 0) return null
  const scale = Math.min(MEDIA_MAX_W / w, MEDIA_MAX_H / h, 1)
  let width = w * scale
  let height = h * scale
  if (width < MEDIA_MIN_W) {
    // Mirrors CSS min-width resolution: widen to the minimum, scale height by
    // the same factor, clamp to max height (object-cover crops the overflow).
    height = Math.min(MEDIA_MAX_H, (height * MEDIA_MIN_W) / width)
    width = MEDIA_MIN_W
  }
  return { width: Math.round(width), height: Math.round(height) }
}

interface MediaContentProps {
  message: store.DecodedMessage
  type: "image" | "video" | "sticker" | "audio" | "document"
  chatId: string
  isGif?: boolean
  sentMediaCache?: React.MutableRefObject<Map<string, string>>
  onImageClick?: (src: string) => void
  onDownload?: () => void
}

export function MediaContent({
  message,
  type,
  chatId,
  isGif,
  sentMediaCache,
  onImageClick,
  onDownload,
}: MediaContentProps) {
  // Seed from the module caches so a remounted row paints its final content
  // immediately instead of placeholder-then-swap.
  const [mediaSrc, setMediaSrc] = useState<string | null>(
    () => imagePathCache.get(message.Info.ID) ?? null,
  )
  const [loading, setLoading] = useState(false)
  const [showDownloadButton, setShowDownloadButton] = useState(false)
  const [thumbnailSrc, setThumbnailSrc] = useState<string | null>(
    () => videoThumbCache.get(message.Info.ID) ?? null,
  )
  const loadingRef = useRef(false)
  const mountedRef = useRef(true)
  const gifLoopsRef = useRef(0)
  const placeholderRef = useRef<HTMLDivElement | null>(null)
  const openLightbox = useUIStore(s => s.openLightbox)

  // Reserve the final layout box before the media loads. Only images and GIF
  // videos swap a placeholder for an inline element, so only they can shift.
  const messageBody = (message.Content as any)?.[`${type}Message`]
  const reservedBox =
    type === "image" || (type === "video" && isGif)
      ? (mediaBox(messageBody?.width, messageBody?.height) ??
        (type === "image" ? IMAGE_FALLBACK_BOX : GIF_FALLBACK_BOX))
      : null

  const handleDownload = async () => {
    if (loadingRef.current) return
    loadingRef.current = true
    setLoading(true)
    try {
      const dataUrl =
        type === "image" || type === "sticker"
          ? await loadMediaOnce(`image:${message.Info.ID}`, () => GetCachedImage(message.Info.ID))
          : await loadMediaOnce(`media:${chatId}:${message.Info.ID}`, () =>
              DownloadMedia(chatId, message.Info.ID),
            )
      if (dataUrl) {
        if (type === "image" || type === "sticker" || isGif) {
          imagePathCache.set(message.Info.ID, dataUrl)
        }
        if (mountedRef.current) setMediaSrc(dataUrl)
      }
    } catch {
      // Keep the download affordance available for a retry.
    } finally {
      loadingRef.current = false
      if (mountedRef.current) setLoading(false)
    }
  }

  // Regular videos play full-screen in the lightbox (keeps the chat thumbnail).
  // The downloaded data URL is cached in a ref so reopening doesn't re-download.
  const videoDataRef = useRef<string>("")
  const openVideo = async () => {
    if (videoDataRef.current) {
      openLightbox(videoDataRef.current, "video")
      return
    }
    if (loadingRef.current) return
    loadingRef.current = true
    setLoading(true)
    try {
      const dataUrl = await loadMediaOnce(`media:${chatId}:${message.Info.ID}`, () =>
        DownloadMedia(chatId, message.Info.ID),
      )
      videoDataRef.current = dataUrl
      openLightbox(dataUrl, "video")
    } catch {
    } finally {
      loadingRef.current = false
      if (mountedRef.current) setLoading(false)
    }
  }

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
    }
  }, [])

  useEffect(() => {
    const content = message.Content as any
    const messageBody = content?.[`${type}Message`]

    if (messageBody?._tempImage) {
      setMediaSrc(messageBody._tempImage)
      return
    }
    if (messageBody?._tempFile) {
      const blobUrl = URL.createObjectURL(messageBody._tempFile)
      setMediaSrc(blobUrl)
      return
    }
    if (sentMediaCache?.current.has(message.Info.ID)) {
      setMediaSrc(sentMediaCache.current.get(message.Info.ID)!)
      return
    }
  }, [message.Content, message.Info.ID, sentMediaCache, type])

  // Auto-download image/sticker media once it's visible on screen — covers both
  // "already visible when the chat opens" and "scrolled into view". Debounced so
  // rows blazed past during a fast scroll don't fetch.
  useEffect(() => {
    const autoLoads = type === "image" || type === "sticker" || (type === "video" && isGif)
    if (mediaSrc || !autoLoads) return
    const el = placeholderRef.current
    if (!el) return
    let timer: ReturnType<typeof setTimeout> | undefined
    const obs = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          timer = setTimeout(() => void handleDownload(), 200)
        } else if (timer) {
          clearTimeout(timer)
          timer = undefined
        }
      },
      { rootMargin: "150px", threshold: 0.01 },
    )
    obs.observe(el)
    return () => {
      obs.disconnect()
      if (timer) clearTimeout(timer)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mediaSrc, type, isGif, message.Info.ID])

  // Fetch the embedded preview for regular videos so the list shows a thumbnail
  // + play button without downloading the whole video. (GIFs auto-play instead.)
  useEffect(() => {
    if (type !== "video" || isGif || mediaSrc || thumbnailSrc) return
    let cancelled = false
    GetVideoThumbnail(message.Info.ID)
      .then(url => {
        if (!url) return
        videoThumbCache.set(message.Info.ID, url)
        if (!cancelled) setThumbnailSrc(url)
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [type, isGif, mediaSrc, thumbnailSrc, message.Info.ID])

  useEffect(() => {
    // Cleanup blob URLs when component unmounts or mediaSrc changes
    return () => {
      if (mediaSrc?.startsWith("blob:")) {
        URL.revokeObjectURL(mediaSrc)
      }
    }
  }, [mediaSrc])

  // Voice notes / audio always render the custom player. When there's no src
  // yet (a received note not downloaded), the play button fetches it first.
  if (type === "audio") {
    return <AudioPlayer src={mediaSrc} loading={loading} onNeedSrc={handleDownload} />
  }

  if (mediaSrc) {
    if (type === "image" || type === "sticker") {
      return (
        <div
          className="relative inline-block"
          onMouseEnter={() => type === "image" && setShowDownloadButton(true)}
          onMouseLeave={() => setShowDownloadButton(false)}
        >
          <img
            src={mediaSrc}
            className={
              type === "image"
                ? "block min-w-75 max-w-82.5 max-h-100 object-cover rounded-lg cursor-pointer"
                : "object-contain w-48.75 h-48.75"
            }
            // Same explicit box as the placeholder -> zero layout shift.
            style={type === "image" ? (reservedBox ?? undefined) : undefined}
            decoding="async"
            alt="media"
            onClick={
              type === "image"
                ? () => {
                    onImageClick?.(mediaSrc)
                    openLightbox(mediaSrc)
                  }
                : undefined
            }
          />
          {type === "image" && showDownloadButton && onDownload && (
            <button
              onClick={e => {
                e.stopPropagation()
                onDownload()
              }}
              className="absolute top-2 right-2 p-2 bg-black/70 hover:bg-black/90 rounded-full text-white transition-colors"
              title="Download image"
            >
              <svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor">
                <path d="M19 9h-4V3H9v6H5l7 7 7-7zM5 18v2h14v-2H5z" />
              </svg>
            </button>
          )}
        </div>
      )
    }
    if (type === "video")
      return isGif ? (
        <video
          src={mediaSrc}
          autoPlay
          muted
          playsInline
          onEnded={e => {
            if (gifLoopsRef.current < MAX_GIF_LOOPS - 1) {
              gifLoopsRef.current += 1
              const v = e.currentTarget
              v.currentTime = 0
              void v.play().catch(() => {})
            }
          }}
          onClick={e => {
            // Replay from the start when clicked, even after it has stopped.
            const v = e.currentTarget
            gifLoopsRef.current = 0
            v.currentTime = 0
            void v.play().catch(() => {})
          }}
          className="block min-w-75 max-w-82.5 max-h-100 rounded-lg cursor-pointer object-cover"
          style={reservedBox ?? GIF_FALLBACK_BOX}
        />
      ) : (
        <video src={mediaSrc} controls className="block w-64 h-64 rounded-lg object-cover" />
      )
  }

  // Video placeholder: show the embedded thumbnail (if any) with a play button
  // so it's clearly a video, and only download the full file on click.
  if (type === "video") {
    return (
      <div
        ref={placeholderRef}
        onClick={openVideo}
        className="relative w-64 h-64 rounded-lg overflow-hidden bg-gray-300 dark:bg-gray-800 flex items-center justify-center cursor-pointer bg-cover bg-center"
        // GIFs auto-swap this placeholder for the inline <video>, so it must
        // already occupy the video's final box when dimensions are known.
        style={{
          ...(isGif ? (reservedBox ?? GIF_FALLBACK_BOX) : null),
          ...(thumbnailSrc ? { backgroundImage: `url(${thumbnailSrc})` } : null),
        }}
      >
        {loading ? (
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-white" />
        ) : (
          <div className="bg-black/55 rounded-full p-3">
            <svg viewBox="0 0 24 24" width="28" height="28" fill="white">
              <path d="M8 5v14l11-7z" />
            </svg>
          </div>
        )}
      </div>
    )
  }

  if (type === "audio") {
    return (
      <div
        ref={placeholderRef}
        className="w-75 h-14 rounded-lg bg-gray-200 dark:bg-gray-800 flex items-center justify-center"
      >
        {loading ? (
          <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-green-500" />
        ) : (
          <button
            onClick={() => void handleDownload()}
            className="bg-black/50 p-2 rounded-full text-white hover:bg-black/70"
          >
            <svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor">
              <path d="M19 9h-4V3H9v6H5l7 7 7-7zM5 18v2h14v-2H5z" />
            </svg>
          </button>
        )}
      </div>
    )
  }

  return (
    <div
      ref={placeholderRef}
      // Stickers always render at a fixed 195px square (w-48.75 above), so
      // reserve exactly that; images get their computed final box when the
      // backend knows the dimensions. Both avoid a resize on load.
      className={`${
        type === "sticker" ? "w-48.75 h-48.75" : "w-64 h-64"
      } bg-gray-200 dark:bg-gray-800 rounded-lg flex items-center justify-center`}
      style={reservedBox ?? undefined}
    >
      {loading ? (
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-green-500" />
      ) : (
        <button
          onClick={() => void handleDownload()}
          className="bg-black/50 p-3 rounded-full text-white hover:bg-black/70"
        >
          <svg viewBox="0 0 24 24" width="24" height="24" fill="currentColor">
            <path d="M19 9h-4V3H9v6H5l7 7 7-7zM5 18v2h14v-2H5z" />
          </svg>
        </button>
      )}
    </div>
  )
}

// A compact WhatsApp-style voice-note player: play/pause, a seekable progress
// bar and a time readout. `onNeedSrc` fetches the audio on first play when it
// hasn't been downloaded yet.
function AudioPlayer({
  src,
  loading,
  onNeedSrc,
}: {
  src: string | null
  loading: boolean
  onNeedSrc: () => void
}) {
  const audioRef = useRef<HTMLAudioElement>(null)
  const wantPlayRef = useRef(false)
  const [playing, setPlaying] = useState(false)
  const [cur, setCur] = useState(0)
  const [dur, setDur] = useState(0)

  // Auto-play once the src arrives if the user pressed play while it loaded.
  useEffect(() => {
    if (src && wantPlayRef.current && audioRef.current) {
      wantPlayRef.current = false
      audioRef.current.play().catch(() => {})
    }
  }, [src])

  const toggle = () => {
    if (!src) {
      wantPlayRef.current = true
      onNeedSrc()
      return
    }
    const a = audioRef.current
    if (!a) return
    if (a.paused) a.play().catch(() => {})
    else a.pause()
  }

  const fmt = (s: number) => {
    if (!isFinite(s) || s < 0) return "0:00"
    const m = Math.floor(s / 60)
    const ss = Math.floor(s % 60)
    return `${m}:${String(ss).padStart(2, "0")}`
  }

  const pct = dur > 0 && isFinite(dur) ? (cur / dur) * 100 : 0

  return (
    <div className="flex items-center gap-3 min-w-56 py-1">
      <button
        onClick={toggle}
        className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-green text-white"
        title={playing ? "Pause" : "Play"}
      >
        {loading ? (
          <span className="h-4 w-4 animate-spin rounded-full border-b-2 border-white" />
        ) : playing ? (
          <svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor">
            <path d="M6 5h4v14H6zM14 5h4v14h-4z" />
          </svg>
        ) : (
          <svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor">
            <path d="M8 5v14l11-7z" />
          </svg>
        )}
      </button>
      <div className="flex-1">
        <div
          className="h-1 w-full cursor-pointer rounded-full bg-black/15 dark:bg-white/20"
          onClick={e => {
            const a = audioRef.current
            if (!a || !dur || !isFinite(dur)) return
            const rect = e.currentTarget.getBoundingClientRect()
            a.currentTime = ((e.clientX - rect.left) / rect.width) * dur
          }}
        >
          <div className="h-full rounded-full bg-green" style={{ width: `${pct}%` }} />
        </div>
        <div className="mt-1 text-[10px] opacity-60">{fmt(cur > 0 || playing ? cur : dur)}</div>
      </div>
      {src && (
        <audio
          ref={audioRef}
          src={src}
          preload="metadata"
          className="hidden"
          onPlay={() => setPlaying(true)}
          onPause={() => setPlaying(false)}
          onEnded={() => {
            setPlaying(false)
            setCur(0)
          }}
          onTimeUpdate={e => setCur(e.currentTarget.currentTime)}
          onLoadedMetadata={e => setDur(e.currentTarget.duration)}
          onDurationChange={e => setDur(e.currentTarget.duration)}
        />
      )}
    </div>
  )
}
