import { Component, type ErrorInfo, type ReactNode } from 'react'
import { FaRotateRight } from 'react-icons/fa6'

interface ErrorBoundaryProps {
  children: ReactNode
  fallback?: ReactNode
}

interface ErrorBoundaryState {
  error: Error | null
}

export class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  state: ErrorBoundaryState = { error: null }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error }
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error('Unhandled frontend error', error, errorInfo)
  }

  render() {
    if (!this.state.error) {
      return this.props.children
    }

    return this.props.fallback ?? <DefaultErrorFallback error={this.state.error} />
  }
}

function DefaultErrorFallback({ error }: { error: Error }) {
  return (
    <main className='app-error-boundary' role='alert'>
      <div className='app-error-boundary__panel'>
        <p className='app-error-boundary__eyebrow'>Frontend Error</p>
        <h1>Something went wrong</h1>
        <p>{error.message || 'The UI hit an unexpected error while rendering this view.'}</p>
        <button type='button' onClick={() => window.location.reload()}>
          <FaRotateRight />
          <span>Reload</span>
        </button>
      </div>
    </main>
  )
}

export default ErrorBoundary
