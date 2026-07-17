import { createRoot } from "react-dom/client"
import "./style.css"
import App from "./App"
import { ErrorBoundary } from "./components/ErrorBoundary"
import { applyThemeClass, readCachedTheme } from "./lib/theme"

// Apply the last known theme before first render so a dark-mode user does not
// see a flash of the light theme while settings load from the backend.
applyThemeClass(readCachedTheme())

const container = document.getElementById("root")

const root = createRoot(container!)

root.render(
  <ErrorBoundary>
    <App />
  </ErrorBoundary>,
)
