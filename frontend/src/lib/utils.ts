import { clsx, type ClassValue } from "clsx"

export function cn(...inputs: ClassValue[]) {
  return clsx(inputs)
}

/**
 * Format a bare phone number (digits, as stored in JIDs) the way WhatsApp
 * displays it: "+<country code> XXXXX XXXXX". Falls back to "+digits" when
 * the number is too short to split.
 */
export function formatPhone(digits: string): string {
  const clean = digits.replace(/\D/g, "")
  if (!clean) return ""
  if (clean.length <= 10) return `+${clean}`
  const cc = clean.slice(0, clean.length - 10)
  const rest = clean.slice(-10)
  return `+${cc} ${rest.slice(0, 5)} ${rest.slice(5)}`
}

/**
 * Extract the phone number from a user JID ("<digits>@s.whatsapp.net").
 * Returns "" for non-phone JIDs (e.g. "@lid" senders).
 */
export function phoneFromJID(jid: string): string {
  if (!jid.endsWith("@s.whatsapp.net")) return ""
  return jid.split("@")[0]
}
