import { describe, expect, it } from "vitest"
import { LRUCache } from "./lruCache"

describe("LRUCache", () => {
  it("evicts the least recently used entry", () => {
    const cache = new LRUCache<string, string>(2)
    cache.set("a", "one").set("b", "two")
    expect(cache.get("a")).toBe("one")

    cache.set("c", "three")
    expect(cache.has("a")).toBe(true)
    expect(cache.has("b")).toBe(false)
    expect(cache.has("c")).toBe(true)
  })

  it("also caps the combined value weight", () => {
    const cache = new LRUCache<string, string>(10, 5, value => value.length)
    cache.set("a", "123").set("b", "45").set("c", "6")

    expect(cache.has("a")).toBe(false)
    expect(cache.weight).toBe(3)
  })

  it("does not retain a value larger than the whole budget", () => {
    const cache = new LRUCache<string, string>(10, 3, value => value.length)
    cache.set("small", "12").set("huge", "1234")

    expect(cache.has("small")).toBe(true)
    expect(cache.has("huge")).toBe(false)
  })

  it("replacing a key updates its weight without growing the entry count", () => {
    const cache = new LRUCache<string, string>(2, 10, value => value.length)
    cache.set("a", "1").set("a", "123")

    expect(cache.size).toBe(1)
    expect(cache.weight).toBe(3)
  })
})
