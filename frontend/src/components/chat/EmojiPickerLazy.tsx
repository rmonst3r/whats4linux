import data from "@emoji-mart/data"
import Picker from "@emoji-mart/react"

export default function EmojiPickerLazy(props: Record<string, unknown>) {
  return <Picker data={data} {...props} />
}
