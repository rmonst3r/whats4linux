import { useEffect, useState } from "react"
import { GetLinkPreview, GetLinkPreviewImage } from "../../../wailsjs/go/api/Api"
import { BrowserOpenURL } from "../../../wailsjs/runtime/runtime"

interface Preview {
  url: string
  title: string
  description: string
  thumbnail: string
}

// Renders the WhatsApp link-preview card under a message, if one was stored.
export function LinkPreview({ messageId }: { messageId: string }) {
  const [preview, setPreview] = useState<Preview | null>(null)
  const [poster, setPoster] = useState<string>("")

  useEffect(() => {
    let cancelled = false
    GetLinkPreview(messageId)
      .then(p => {
        if (cancelled || !p || (!p.title && !p.thumbnail)) return
        setPreview(p as Preview)
        // The poster is usually a downloadable reference, not embedded — fetch
        // it lazily (downloaded + cached on first request).
        if (!p.thumbnail) {
          GetLinkPreviewImage(messageId)
            .then(url => {
              if (!cancelled && url) setPoster(url)
            })
            .catch(() => {})
        }
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [messageId])

  if (!preview) return null

  const image = preview.thumbnail || poster

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
      {image && <img src={image} alt="" className="w-full max-h-64 object-contain bg-black/20" />}
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
