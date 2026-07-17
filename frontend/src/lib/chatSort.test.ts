import { describe, it, expect } from "vitest"
import { sortChatItems } from "./chatSort"
import type { ChatItem } from "../store/types"

const chat = (id: string, timestamp: number, pinned = false): ChatItem => ({
  id,
  name: id,
  subtitle: "",
  type: "contact",
  timestamp,
  pinned,
})

describe("sortChatItems", () => {
  it("orders by last-message time when nothing is pinned", () => {
    const out = sortChatItems([chat("a", 1), chat("b", 3), chat("c", 2)])
    expect(out.map(c => c.id)).toEqual(["b", "c", "a"])
  })

  it("keeps pinned chats above unpinned regardless of recency", () => {
    const out = sortChatItems([chat("new", 100), chat("pinned-old", 1, true)])
    expect(out.map(c => c.id)).toEqual(["pinned-old", "new"])
  })

  it("orders within the pinned block by recency", () => {
    const out = sortChatItems([
      chat("p1", 5, true),
      chat("x", 50),
      chat("p2", 9, true),
    ])
    expect(out.map(c => c.id)).toEqual(["p2", "p1", "x"])
  })

  it("handles missing timestamps as oldest", () => {
    const noTs: ChatItem = { id: "n", name: "n", subtitle: "", type: "contact" }
    const out = sortChatItems([noTs, chat("a", 1)])
    expect(out.map(c => c.id)).toEqual(["a", "n"])
  })

  it("does not mutate the input array", () => {
    const input = [chat("a", 1), chat("b", 2, true)]
    const copy = [...input]
    sortChatItems(input)
    expect(input).toEqual(copy)
  })

  it("handles an empty list", () => {
    expect(sortChatItems([])).toEqual([])
  })
})
