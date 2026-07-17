import { describe, it, expect, vi } from "vitest"
import {
  THEME_STORAGE_KEY,
  applyThemeClass,
  cacheTheme,
  normalizeTheme,
  readCachedTheme,
} from "./theme"

describe("normalizeTheme", () => {
  it("returns dark for 'dark'", () => {
    expect(normalizeTheme("dark")).toBe("dark")
  })

  it("returns light for 'light'", () => {
    expect(normalizeTheme("light")).toBe("light")
  })

  it("defaults to light for null and undefined", () => {
    expect(normalizeTheme(null)).toBe("light")
    expect(normalizeTheme(undefined)).toBe("light")
  })

  it("defaults to light for arbitrary garbage values", () => {
    expect(normalizeTheme("")).toBe("light")
    expect(normalizeTheme("DARK")).toBe("light")
    expect(normalizeTheme("darkness")).toBe("light")
    expect(normalizeTheme(42)).toBe("light")
    expect(normalizeTheme({})).toBe("light")
    expect(normalizeTheme(true)).toBe("light")
  })
})

describe("readCachedTheme", () => {
  it("returns dark when 'dark' is cached", () => {
    const storage = { getItem: vi.fn().mockReturnValue("dark") }
    expect(readCachedTheme(storage)).toBe("dark")
    expect(storage.getItem).toHaveBeenCalledWith(THEME_STORAGE_KEY)
  })

  it("returns light when 'light' is cached", () => {
    const storage = { getItem: () => "light" }
    expect(readCachedTheme(storage)).toBe("light")
  })

  it("returns light when nothing is cached", () => {
    const storage = { getItem: () => null }
    expect(readCachedTheme(storage)).toBe("light")
  })

  it("returns light when the cached value is corrupt", () => {
    const storage = { getItem: () => "banana" }
    expect(readCachedTheme(storage)).toBe("light")
  })

  it("returns light when storage access throws", () => {
    const storage = {
      getItem: () => {
        throw new Error("storage disabled")
      },
    }
    expect(readCachedTheme(storage)).toBe("light")
  })
})

describe("cacheTheme", () => {
  it("writes the theme under the storage key", () => {
    const storage = { setItem: vi.fn() }
    cacheTheme("dark", storage)
    expect(storage.setItem).toHaveBeenCalledWith(THEME_STORAGE_KEY, "dark")

    cacheTheme("light", storage)
    expect(storage.setItem).toHaveBeenCalledWith(THEME_STORAGE_KEY, "light")
  })

  it("swallows storage errors (quota exceeded, disabled storage)", () => {
    const storage = {
      setItem: () => {
        throw new Error("QuotaExceededError")
      },
    }
    expect(() => cacheTheme("dark", storage)).not.toThrow()
  })
})

describe("applyThemeClass", () => {
  function fakeRoot() {
    return {
      classList: {
        add: vi.fn(),
        remove: vi.fn(),
      },
    }
  }

  it("adds the dark class for the dark theme", () => {
    const root = fakeRoot()
    applyThemeClass("dark", root)
    expect(root.classList.add).toHaveBeenCalledWith("dark")
    expect(root.classList.remove).not.toHaveBeenCalled()
  })

  it("removes the dark class for the light theme", () => {
    const root = fakeRoot()
    applyThemeClass("light", root)
    expect(root.classList.remove).toHaveBeenCalledWith("dark")
    expect(root.classList.add).not.toHaveBeenCalled()
  })
})
