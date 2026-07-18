import { useCallback, useEffect, useRef, useState } from "react"
import clsx from "clsx"
import { GetCommunityList, GetCommunityDetails, GetCachedAvatar } from "../../../wailsjs/go/api/Api"
import { api } from "../../../wailsjs/go/models"
import { GoBackIcon } from "../../assets/svgs/header_icons"
import { getAvatarColor, AVATAR_ICON_COLOR } from "../../lib/utils"
import { useAppSettingsStore } from "../../store/useAppSettingsStore"

/** Communities empty-state illustration. */
const CommunitiesEmptyIcon = () => (
  <svg
    viewBox="0 0 24 24"
    width="96"
    height="96"
    className="fill-current text-gray-300 dark:text-gray-600"
  >
    <path d="M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z" />
  </svg>
)

const CommunityPeopleIcon = ({ size }: { size: "sm" | "md" | "lg" }) => {
  const px = size === "lg" ? 36 : size === "sm" ? 20 : 24
  return (
    <svg viewBox="0 0 24 24" width={px} height={px} fill={AVATAR_ICON_COLOR} aria-hidden>
      <path d="M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z" />
    </svg>
  )
}

const CommunityAvatar = ({
  avatar,
  name,
  jid,
  size = "md",
}: {
  avatar?: string
  name: string
  jid?: string
  size?: "sm" | "md" | "lg"
}) => {
  const theme = useAppSettingsStore(s => s.theme)
  const dark = theme === "dark"
  // Rounded square — WhatsApp community badge shape.
  const sizeClass =
    size === "lg"
      ? "w-20 h-20 rounded-[22px]"
      : size === "sm"
        ? "w-10 h-10 rounded-[10px]"
        : "w-12 h-12 rounded-[12px]"

  if (avatar) {
    return <img src={avatar} alt={name} className={clsx(sizeClass, "object-cover shrink-0")} />
  }

  const bg = getAvatarColor(jid || name, dark)

  return (
    <div
      className={clsx(sizeClass, "shrink-0 flex items-center justify-center")}
      style={{ backgroundColor: bg }}
    >
      <CommunityPeopleIcon size={size} />
    </div>
  )
}

interface CommunityListProps {
  searchTerm: string
  selectedJid: string | null
  onSelect: (community: api.CommunitySummary) => void
}

/** Sidebar list of communities the user belongs to. */
export function CommunityList({ searchTerm, selectedJid, onSelect }: CommunityListProps) {
  const [communities, setCommunities] = useState<api.CommunitySummary[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const loadGeneration = useRef(0)

  const load = useCallback(async () => {
    const generation = ++loadGeneration.current
    setLoading(true)
    setError(null)
    try {
      const list = (await GetCommunityList()) || []
      if (generation !== loadGeneration.current) return
      setCommunities(list)
      setLoading(false)

      // Render the list first, then hydrate avatars with bounded concurrency.
      const pending = list.filter(c => !c.avatar_url)
      let next = 0
      const worker = async () => {
        while (next < pending.length && generation === loadGeneration.current) {
          const c = pending[next++]
          try {
            const url = await GetCachedAvatar(c.jid, false)
            if (url && generation === loadGeneration.current) {
              setCommunities(prev =>
                prev.map(x => (x.jid === c.jid ? { ...x, avatar_url: url } : x)),
              )
            }
          } catch {
            /* ignore missing avatars */
          }
        }
      }
      void Promise.all(Array.from({ length: Math.min(4, pending.length) }, () => worker()))
    } catch (err) {
      if (generation !== loadGeneration.current) return
      console.error("Failed to load communities:", err)
      setError("Could not load communities")
      setCommunities([])
    } finally {
      if (generation === loadGeneration.current) setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
    return () => {
      loadGeneration.current++
    }
  }, [load])

  const filtered = searchTerm
    ? communities.filter(c => c.name.toLowerCase().includes(searchTerm.toLowerCase()))
    : communities

  if (loading) {
    return <div className="p-6 text-center text-sm text-gray-500">Loading communities…</div>
  }

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center p-8 text-gray-500 dark:text-gray-400">
        <p className="text-center text-sm">{error}</p>
        <button
          onClick={load}
          className="mt-4 px-4 py-2 bg-green-500 text-white rounded-lg hover:bg-green-600 text-sm"
        >
          Retry
        </button>
      </div>
    )
  }

  if (filtered.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center p-8 text-gray-500 dark:text-gray-400">
        <CommunitiesEmptyIcon />
        <p className="mt-4 text-center text-sm">
          {searchTerm ? "No communities match your search." : "You're not in any communities yet."}
        </p>
        <p className="mt-2 text-center text-xs text-gray-400 dark:text-[#8696a0] max-w-[240px]">
          Communities bring related groups together with a shared announcement channel.
        </p>
        {!searchTerm && (
          <button
            onClick={load}
            className="mt-4 px-4 py-2 bg-green-500 text-white rounded-lg hover:bg-green-600 text-sm"
          >
            Refresh
          </button>
        )}
      </div>
    )
  }

  return (
    <div>
      {filtered.map(c => (
        <button
          key={c.jid}
          type="button"
          onClick={() => onSelect(c)}
          className={clsx(
            "flex w-full items-center px-4 py-3 text-left cursor-pointer",
            "hover:bg-gray-100 dark:hover:bg-[#202121]",
            selectedJid === c.jid && "bg-gray-200 dark:bg-[#2e2f2f]",
          )}
        >
          <CommunityAvatar avatar={c.avatar_url} name={c.name} jid={c.jid} />
          <div className="ml-4 flex-1 min-w-0">
            <h3 className="text-light-text dark:text-dark-text font-medium truncate">{c.name}</h3>
            <p className="text-sm text-gray-500 dark:text-[#8696a0] truncate">
              {c.group_count > 0
                ? `${c.group_count} group${c.group_count === 1 ? "" : "s"}`
                : c.topic || "Community"}
            </p>
          </div>
        </button>
      ))}
    </div>
  )
}

interface CommunityHomeProps {
  communityJid: string
  communityName: string
  communityAvatar?: string
  onBack: () => void
  onOpenGroup: (jid: string, name: string, avatar?: string) => void
}

/** Community home: header, announcements, and linked groups. */
export function CommunityHome({
  communityJid,
  communityName,
  communityAvatar,
  onBack,
  onOpenGroup,
}: CommunityHomeProps) {
  const theme = useAppSettingsStore(s => s.theme)
  const dark = theme === "dark"
  const [details, setDetails] = useState<api.CommunityDetails | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)
    setDetails(null)
    ;(async () => {
      try {
        const d = await GetCommunityDetails(communityJid)
        if (!cancelled) {
          setDetails(d)
          setLoading(false)

          const targets = [
            { kind: "community" as const, jid: communityJid },
            ...(d.announcement ? [{ kind: "announcement" as const, jid: d.announcement.jid }] : []),
            ...(d.groups || []).map(group => ({ kind: "group" as const, jid: group.jid })),
          ]
          let next = 0
          const worker = async () => {
            while (next < targets.length && !cancelled) {
              const target = targets[next++]
              try {
                const url = await GetCachedAvatar(target.jid, false)
                if (!url || cancelled) continue
                setDetails(current => {
                  if (!current) return current
                  if (target.kind === "community") return { ...current, avatar_url: url }
                  if (target.kind === "announcement") {
                    return current.announcement?.jid === target.jid
                      ? { ...current, announcement: { ...current.announcement, avatar_url: url } }
                      : current
                  }
                  return {
                    ...current,
                    groups: current.groups.map(group =>
                      group.jid === target.jid ? { ...group, avatar_url: url } : group,
                    ),
                  }
                })
              } catch {
                /* ignore missing avatars */
              }
            }
          }
          void Promise.all(Array.from({ length: Math.min(4, targets.length) }, () => worker()))
        }
      } catch (err) {
        console.error("Failed to load community:", err)
        if (!cancelled) {
          setError("Could not load community details")
          setLoading(false)
        }
      }
    })()
    return () => {
      cancelled = true
    }
  }, [communityJid])

  const name = details?.name || communityName
  const avatar = details?.avatar_url || communityAvatar
  const topic = details?.topic
  const memberCount = details?.member_count ?? 0
  const announcement = details?.announcement
  const groups = details?.groups ?? []

  return (
    <div className="flex flex-col h-full bg-light-secondary dark:bg-dark-bg">
      {/* Top bar */}
      <div className="h-16 bg-light-secondary dark:bg-dark-bg flex items-center gap-3 px-4 border-b border-gray-200 dark:border-white/5 shrink-0">
        <button
          type="button"
          onClick={onBack}
          className="p-2 hover:bg-gray-200 dark:hover:bg-white/10 rounded-full transition-colors"
          aria-label="Back"
        >
          <GoBackIcon />
        </button>
        <CommunityAvatar avatar={avatar} name={name} jid={communityJid} size="sm" />
        <div className="min-w-0 flex-1">
          <h2 className="font-medium text-light-text dark:text-dark-text truncate">{name}</h2>
          {memberCount > 0 && (
            <p className="text-xs text-gray-500 dark:text-[#8696a0]">
              {memberCount.toLocaleString()} member{memberCount === 1 ? "" : "s"}
            </p>
          )}
        </div>
      </div>

      <div className="flex-1 overflow-y-auto">
        {/* Hero / community info card */}
        <div className="flex flex-col items-center px-6 pt-8 pb-6 bg-white dark:bg-dark-secondary">
          <CommunityAvatar avatar={avatar} name={name} jid={communityJid} size="lg" />
          <h1 className="mt-4 text-xl font-medium text-light-text dark:text-dark-text text-center">
            {name}
          </h1>
          {topic ? (
            <p className="mt-2 text-sm text-gray-500 dark:text-[#8696a0] text-center max-w-md whitespace-pre-wrap">
              {topic}
            </p>
          ) : (
            <p className="mt-2 text-sm text-gray-400 dark:text-[#8696a0] text-center">
              Community · {memberCount > 0 ? `${memberCount.toLocaleString()} members` : "Groups"}
            </p>
          )}
        </div>

        {loading && <div className="p-6 text-center text-sm text-gray-500">Loading groups…</div>}

        {error && (
          <div className="p-6 text-center text-sm text-red-500 dark:text-red-400">{error}</div>
        )}

        {!loading && !error && (
          <>
            {/* Announcements */}
            {announcement && (
              <section className="mt-2 bg-white dark:bg-dark-secondary">
                <div className="px-4 py-2 text-xs font-medium uppercase tracking-wide text-[#008069] dark:text-[#21c063]">
                  Announcements
                </div>
                <button
                  type="button"
                  onClick={() =>
                    onOpenGroup(
                      announcement.jid,
                      announcement.name || "Announcements",
                      announcement.avatar_url,
                    )
                  }
                  className="flex w-full items-center px-4 py-3 hover:bg-gray-100 dark:hover:bg-[#202121] text-left"
                >
                  <div
                    className="w-12 h-12 rounded-[12px] flex items-center justify-center shrink-0 overflow-hidden"
                    style={{ backgroundColor: getAvatarColor(announcement.jid, dark) }}
                  >
                    {announcement.avatar_url ? (
                      <img
                        src={announcement.avatar_url}
                        alt=""
                        className="w-full h-full object-cover"
                      />
                    ) : (
                      <svg
                        viewBox="0 0 24 24"
                        width="22"
                        height="22"
                        fill={AVATAR_ICON_COLOR}
                        aria-hidden
                      >
                        <path d="M12 3v10.55c-.59-.34-1.27-.55-2-.55-2.21 0-4 1.79-4 4s1.79 4 4 4 4-1.79 4-4V7h4V3h-6z" />
                      </svg>
                    )}
                  </div>
                  <div className="ml-4 min-w-0 flex-1">
                    <h3 className="font-medium text-light-text dark:text-dark-text truncate">
                      {announcement.name || "Announcements"}
                    </h3>
                    <p className="text-sm text-gray-500 dark:text-[#8696a0] truncate">
                      Only admins can send messages
                    </p>
                  </div>
                </button>
              </section>
            )}

            {/* Groups in community */}
            <section className="mt-2 bg-white dark:bg-dark-secondary pb-4">
              <div className="px-4 py-2 text-xs font-medium uppercase tracking-wide text-[#008069] dark:text-[#21c063]">
                Groups{groups.length > 0 ? ` · ${groups.length}` : ""}
              </div>

              {groups.length === 0 && !announcement ? (
                <p className="px-4 py-6 text-sm text-gray-500 dark:text-[#8696a0] text-center">
                  No groups in this community yet.
                </p>
              ) : groups.length === 0 ? (
                <p className="px-4 py-4 text-sm text-gray-500 dark:text-[#8696a0]">
                  No other groups linked.
                </p>
              ) : (
                groups.map(g => (
                  <button
                    key={g.jid}
                    type="button"
                    onClick={() => onOpenGroup(g.jid, g.name, g.avatar_url)}
                    className="flex w-full items-center px-4 py-3 hover:bg-gray-100 dark:hover:bg-[#202121] text-left"
                  >
                    <div
                      className="w-12 h-12 rounded-full overflow-hidden flex items-center justify-center shrink-0"
                      style={
                        g.avatar_url ? undefined : { backgroundColor: getAvatarColor(g.jid, dark) }
                      }
                    >
                      {g.avatar_url ? (
                        <img src={g.avatar_url} alt="" className="w-full h-full object-cover" />
                      ) : (
                        <svg
                          viewBox="0 0 24 24"
                          width="26"
                          height="26"
                          fill={AVATAR_ICON_COLOR}
                          aria-hidden
                        >
                          <path d="M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z" />
                        </svg>
                      )}
                    </div>
                    <div className="ml-4 min-w-0 flex-1">
                      <h3 className="font-medium text-light-text dark:text-dark-text truncate">
                        {g.name || "Group"}
                      </h3>
                      <p className="text-sm text-gray-500 dark:text-[#8696a0] truncate">Group</p>
                    </div>
                  </button>
                ))
              )}
            </section>
          </>
        )}
      </div>
    </div>
  )
}

/** Welcome panel when Communities tab is active but nothing is selected. */
export function CommunitiesWelcome() {
  return (
    <div className="flex-1 flex flex-col items-center justify-center z-10 text-center px-10 border-b-[6px] border-[#43d187]">
      <div className="mb-6">
        <CommunitiesEmptyIcon />
      </div>
      <h1 className="text-3xl font-light text-gray-600 dark:text-gray-300 mb-4">Communities</h1>
      <p className="text-gray-500 dark:text-gray-400 max-w-md">
        Communities bring members together in topic-based groups and let admins send announcements
        to everyone.
      </p>
      <p className="mt-4 text-sm text-gray-400 dark:text-[#8696a0] max-w-sm">
        Select a community from the list to view its announcement channel and groups.
      </p>
    </div>
  )
}
