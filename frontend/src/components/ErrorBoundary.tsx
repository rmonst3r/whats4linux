import { Component, type ErrorInfo, type ReactNode } from "react"

interface ErrorBoundaryProps {
  children: ReactNode
}

interface ErrorBoundaryState {
  error: Error | null
}

/**
 * Top-level error boundary. Without it, any render-time error unmounts the
 * whole React tree and leaves a blank white window (see the missing-useState
 * regression). This renders a recoverable fallback instead.
 */
export class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  state: ErrorBoundaryState = { error: null }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("Uncaught render error:", error, info.componentStack)
  }

  handleReload = () => {
    window.location.reload()
  }

  handleReset = () => {
    this.setState({ error: null })
  }

  render() {
    const { error } = this.state

    if (!error) return this.props.children

    return (
      <div className="min-h-screen flex flex-col items-center justify-center gap-4 bg-light-secondary text-light-text dark:bg-black dark:text-white p-8">
        <h1 className="text-2xl font-semibold">Something went wrong</h1>
        <p className="text-gray-500 dark:text-gray-400 text-center max-w-md">
          The app hit an unexpected error and could not continue rendering.
        </p>
        <pre className="max-w-full overflow-auto rounded bg-gray-100 dark:bg-dark-tertiary px-4 py-2 text-xs text-red-600 dark:text-red-400">
          {error.message}
        </pre>
        <div className="flex gap-3">
          <button
            onClick={this.handleReset}
            className="px-4 py-2 rounded-lg border border-gray-300 dark:border-dark-tertiary hover:bg-gray-100 dark:hover:bg-dark-tertiary"
          >
            Try again
          </button>
          <button
            onClick={this.handleReload}
            className="px-4 py-2 rounded-lg bg-green-500 text-white hover:bg-green-600"
          >
            Reload app
          </button>
        </div>
      </div>
    )
  }
}
