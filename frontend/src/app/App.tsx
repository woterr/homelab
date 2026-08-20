import { Component, lazy, Suspense, type ComponentType, type ReactNode } from 'react'
import { Routes, Route, Navigate } from 'react-router-dom'
import { Shell } from './shell/Shell'
import { Empty, Spinner } from '../components/ui/Feedback'

const lazyPage = (loader: () => Promise<Record<string, unknown>>, name: string) =>
  lazy(() => loader().then((m) => ({ default: m[name] as ComponentType })))

const OverviewPage = lazyPage(() => import('../features/overview/OverviewPage'), 'OverviewPage')
const MetricsPage = lazyPage(() => import('../features/metrics/MetricsPage'), 'MetricsPage')
const NetworkPage = lazyPage(() => import('../features/network/NetworkPage'), 'NetworkPage')
const StoragePage = lazyPage(() => import('../features/storage/StoragePage'), 'StoragePage')
const LogsPage = lazyPage(() => import('../features/logs/LogsPage'), 'LogsPage')
const EventsPage = lazyPage(() => import('../features/events/EventsPage'), 'EventsPage')
const InternetPage = lazyPage(() => import('../features/internet/InternetPage'), 'InternetPage')
const RulesPage = lazyPage(() => import('../features/rules/RulesPage'), 'RulesPage')
const SettingsPage = lazyPage(() => import('../features/settings/SettingsPage'), 'SettingsPage')

function PageLoader() {
  return (
    <div className="page" style={{ alignItems: 'center', paddingTop: 'var(--sp-12)' }}>
      <Spinner size={28} />
    </div>
  )
}

// ErrorBoundary catches render/load failures in a page so one broken route
// (e.g. a failed lazy chunk load) can never blank the whole console.
interface BoundaryState {
  error: Error | null
  path: string
}
class ErrorBoundary extends Component<{ children: ReactNode }, BoundaryState> {
  state: BoundaryState = { error: null, path: window.location.pathname }

  static getDerivedStateFromError(error: Error): Partial<BoundaryState> {
    return { error }
  }

  componentDidUpdate(_prev: unknown, prevState: BoundaryState) {
    if (prevState.error && window.location.pathname !== prevState.path) {
      this.setState({ error: null, path: window.location.pathname })
    }
  }

  componentDidCatch(error: Error) {
    console.error('page crashed:', error)
  }

  render() {
    if (this.state.error) {
      return (
        <div className="page" style={{ alignItems: 'center', paddingTop: 'var(--sp-12)' }}>
          <Empty
            title="This page hit an error"
            hint={this.state.error.message}
            action={
              <button className="btn btn--primary btn--sm" onClick={() => window.location.reload()}>
                Reload
              </button>
            }
          />
        </div>
      )
    }
    return this.props.children
  }
}

export default function App() {
  return (
    <ErrorBoundary>
      <Suspense fallback={<PageLoader />}>
        <Routes>
          <Route element={<Shell />}>
            <Route path="/" element={<OverviewPage />} />
            <Route path="/metrics" element={<MetricsPage />} />
            <Route path="/telemetry" element={<Navigate to="/metrics" replace />} />
            <Route path="/network" element={<NetworkPage />} />
            <Route path="/storage" element={<StoragePage />} />
            <Route path="/logs" element={<LogsPage />} />
            <Route path="/events" element={<EventsPage />} />
            <Route path="/alerts" element={<Navigate to="/events" replace />} />
            <Route path="/internet" element={<InternetPage />} />
            <Route path="/rules" element={<RulesPage />} />
            <Route path="/settings" element={<SettingsPage />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Route>
        </Routes>
      </Suspense>
    </ErrorBoundary>
  )
}