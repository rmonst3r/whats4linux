export type Theme = "light" | "dark"

export const THEME_STORAGE_KEY = "app-theme"

export interface ThemeRoot {
  classList: {
    add: (cls: string) => void
    remove: (cls: string) => void
  }
}

/** Coerces any persisted/unknown value to a valid theme, defaulting to light. */
export function normalizeTheme(value: unknown): Theme {
  return value === "dark" ? "dark" : "light"
}

/**
 * Reads the locally cached theme hint. The backend settings store remains the
 * source of truth; this cache only exists so the correct theme can be applied
 * before the async settings round-trip completes (no flash of the wrong theme).
 */
export function readCachedTheme(storage?: Pick<Storage, "getItem">): Theme {
  try {
    const s = storage ?? window.localStorage
    return normalizeTheme(s.getItem(THEME_STORAGE_KEY))
  } catch {
    return "light"
  }
}

/** Caches the theme locally so the next launch can apply it before first paint. */
export function cacheTheme(theme: Theme, storage?: Pick<Storage, "setItem">): void {
  try {
    const s = storage ?? window.localStorage
    s.setItem(THEME_STORAGE_KEY, theme)
  } catch {
    // Storage unavailable; the backend settings store still persists the theme.
  }
}

/** Applies or removes the Tailwind `dark` class on the document root. */
export function applyThemeClass(theme: Theme, root?: ThemeRoot): void {
  const el = root ?? document.documentElement
  if (theme === "dark") {
    el.classList.add("dark")
  } else {
    el.classList.remove("dark")
  }
}
