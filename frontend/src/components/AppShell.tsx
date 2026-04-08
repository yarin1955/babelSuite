import type { IconType } from 'react-icons'
import type { ReactNode } from 'react'
import { useEffect, useState } from 'react'
import {
  FaArrowLeft,
  FaArrowRight,
  FaBook,
  FaBoxesStacked,
  FaDocker,
  FaFlask,
  FaFolderTree,
  FaGear,
  FaRightFromBracket,
} from 'react-icons/fa6'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { clearSession, getSession } from '../lib/api'
import './AppShell.css'

interface AppShellProps {
  section: string
  sectionTo?: string
  title: string
  description: string
  actions?: ReactNode
  children: ReactNode
}

interface NavItem {
  to: string
  label: string
  icon: IconType
  matchPrefixes: string[]
  adminOnly?: boolean
}

const NAV_ITEMS: NavItem[] = [
  { to: '/', label: 'Runs', icon: FaBoxesStacked, matchPrefixes: ['/', '/executions'] },
  { to: '/sandbox', label: 'Sandbox', icon: FaFlask, matchPrefixes: ['/sandbox'] },
  { to: '/catalog', label: 'Catalog', icon: FaBook, matchPrefixes: ['/catalog'] },
  { to: '/profiles', label: 'Profiles', icon: FaFolderTree, matchPrefixes: ['/profiles'] },
  { to: '/environments', label: 'Environments', icon: FaDocker, matchPrefixes: ['/environments', '/sandboxes'] },
  { to: '/settings', label: 'Settings', icon: FaGear, matchPrefixes: ['/settings'], adminOnly: true },
]

const SIDEBAR_KEY = 'babelsuite.sidebar.collapsed'

export default function AppShell({ section, sectionTo, title, description, actions, children }: AppShellProps) {
  const navigate = useNavigate()
  const location = useLocation()
  const session = getSession()
  const [collapsed, setCollapsed] = useState(() => window.localStorage.getItem(SIDEBAR_KEY) === 'true')
  const visibleNavItems = NAV_ITEMS.filter((item) => !item.adminOnly || session?.user.isAdmin)

  useEffect(() => {
    window.localStorage.setItem(SIDEBAR_KEY, String(collapsed))
  }, [collapsed])

  return (
    <div className={`app-shell${collapsed ? ' app-shell--collapsed' : ''}`}>
      <aside className='app-shell__sidebar'>
        <div className='app-shell__sidebar-inner'>
          <div className='app-shell__brand-row'>
            <button type='button' className='app-shell__brand' onClick={() => navigate('/')}>
              <span className='app-shell__brand-mark'>BS</span>
              <span className='app-shell__brand-copy'>
                <strong>BabelSuite</strong>
                <small>v0.1.0</small>
              </span>
            </button>

            <button
              type='button'
              className='app-shell__collapse-button'
              onClick={() => setCollapsed((current) => !current)}
              aria-label={collapsed ? 'Expand navigation' : 'Collapse navigation'}
              title={collapsed ? 'Expand navigation' : 'Collapse navigation'}
            >
              {collapsed ? <FaArrowRight /> : <FaArrowLeft />}
            </button>
          </div>

          <nav className='app-shell__nav' aria-label='Primary navigation'>
            {visibleNavItems.map((item) => {
              const Icon = item.icon
              const isActive = item.matchPrefixes.some((prefix) => (
                prefix === '/'
                  ? location.pathname === '/' || location.pathname.startsWith('/executions/')
                  : location.pathname === prefix || location.pathname.startsWith(`${prefix}/`)
              ))
              return (
                <Link
                  key={item.to}
                  to={item.to}
                  className={`app-shell__nav-item${isActive ? ' app-shell__nav-item--active' : ''}`}
                  title={item.label}
                >
                  <Icon />
                  <span>{item.label}</span>
                </Link>
              )
            })}
          </nav>

          <div className='app-shell__sidebar-footer'>
            <button
              type='button'
              className='app-shell__signout'
              onClick={() => {
                clearSession()
                navigate('/sign-in')
              }}
            >
              <FaRightFromBracket />
              <span>Sign out</span>
            </button>
          </div>
        </div>
      </aside>

      <div className='app-shell__content'>
        <header className='app-shell__topbar'>
          <div className='app-shell__heading'>
            {sectionTo
              ? <Link to={sectionTo} className='app-shell__eyebrow app-shell__eyebrow--link'>{section}</Link>
              : <p className='app-shell__eyebrow'>{section}</p>
            }
            <h1>{title}</h1>
            <p>{description}</p>
          </div>

          {actions && <div className='app-shell__actions'>{actions}</div>}
        </header>

        <main className='app-shell__body'>{children}</main>
      </div>
    </div>
  )
}
