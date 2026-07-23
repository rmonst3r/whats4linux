import { useState } from "react"
import { Logout } from "../../../wailsjs/go/api/Api"

// Log out unlinks this device from the WhatsApp account (same as removing it
// from Linked Devices on the phone). The backend quits the app on success; the
// next launch shows a fresh QR code to re-link.
const LogOut = () => {
  const [state, setState] = useState<"idle" | "confirm" | "working" | "done">("idle")
  const [error, setError] = useState("")

  const handleLogout = async () => {
    setState("working")
    setError("")
    try {
      await Logout()
      setState("done")
    } catch (err) {
      setError(String(err))
      setState("idle")
    }
  }

  if (state === "done") {
    return (
      <div className="flex flex-col gap-2 text-gray-800 dark:text-gray-200">
        <h2 className="text-lg font-semibold">Logged out</h2>
        <p className="text-sm text-gray-500 dark:text-gray-400">
          This device has been unlinked. The app will close now — reopen it to scan a new QR code.
        </p>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-4 text-gray-800 dark:text-gray-200">
      <div>
        <h2 className="text-lg font-semibold">Log out</h2>
        <p className="text-sm text-gray-500 dark:text-gray-400">
          Unlinks this device from your WhatsApp account and deletes the local session. The app
          will close, and the next launch will show a QR code so you can link again.
        </p>
      </div>

      {error && <p className="text-sm text-red-500">Logout failed: {error}</p>}

      {state === "confirm" ? (
        <div className="flex items-center gap-3">
          <button
            onClick={handleLogout}
            className="px-4 py-2 rounded-lg bg-red-600 hover:bg-red-700 text-white text-sm font-semibold"
          >
            Yes, log out
          </button>
          <button
            onClick={() => setState("idle")}
            className="px-4 py-2 rounded-lg bg-gray-200 dark:bg-gray-700 hover:bg-gray-300 dark:hover:bg-gray-600 text-sm font-semibold"
          >
            Cancel
          </button>
        </div>
      ) : (
        <button
          onClick={() => setState("confirm")}
          disabled={state === "working"}
          className="self-start px-4 py-2 rounded-lg bg-red-600 hover:bg-red-700 disabled:opacity-50 text-white text-sm font-semibold"
        >
          {state === "working" ? "Logging out…" : "Log out"}
        </button>
      )}
    </div>
  )
}

export default LogOut
