import { useDeferredValue, useEffect, useMemo, useRef, useState } from 'react'
import {
  FaBoxArchive,
  FaGear,
  FaMagnifyingGlass,
} from 'react-icons/fa6'
import { useNavigate } from 'react-router-dom'
import AppShell from '../components/AppShell'
import { CatalogInspectModal } from '../components/CatalogInspectModal'
import { PackageCard } from '../components/PackageCard'
import {
  addCatalogFavorite,
  getSession,
  getSuite,
  listCatalogPackages,
  removeCatalogFavorite,
  type CatalogPackage,
  type SuiteDefinition,
} from '../lib/api'
import { buildSuiteDropRef, SANDBOX_SUITE_REF_MIME } from '../lib/sandboxRef'
import './Catalog.css'

type SortKey = 'starred' | 'title' | 'version'

const CAPABILITY_FILTERS = [
  { id: 'openapi', label: 'REST/OpenAPI', terms: ['openapi', 'rest api', 'rest'] },
  { id: 'grpc', label: 'gRPC', terms: ['grpc', 'protobuf', 'proto'] },
  { id: 'soap', label: 'SOAP', terms: ['soap', 'wsdl', 'xml envelope'] },
  { id: 'graphql', label: 'GraphQL', terms: ['graphql'] },
  { id: 'async', label: 'Async', terms: ['async', 'event stream', 'event-driven'] },
  { id: 'kafka', label: 'Kafka', terms: ['kafka'] },
  { id: 'mqtt', label: 'MQTT', terms: ['mqtt'] },
  { id: 'websocket', label: 'WebSocket', terms: ['websocket', 'ws '] },
  { id: 'redis', label: 'Redis', terms: ['redis'] },
  { id: 'postgres', label: 'Postgres', terms: ['postgres'] },
  { id: 'wiremock', label: 'Wiremock', terms: ['wiremock'] },
  { id: 'mock-api', label: 'Mock API', terms: ['mock-api', 'mock api'] },
  { id: 'playwright', label: 'Playwright', terms: ['playwright'] },
  { id: 'vault', label: 'Vault', terms: ['vault'] },
  { id: 'prometheus', label: 'Prometheus', terms: ['prometheus'] },
] as const

export default function Catalog() {
  const navigate = useNavigate()
  const searchRef = useRef<HTMLInputElement>(null)
  const [tab, setTab] = useState<'suite' | 'stdlib'>('suite')
  const [packages, setPackages] = useState<CatalogPackage[]>([])
  const [query, setQuery] = useState('')
  const [provider, setProvider] = useState('All')
  const [activeCapabilities, setActiveCapabilities] = useState<Set<string>>(new Set())
  const [sort, setSort] = useState<SortKey>('starred')
  const [copiedId, setCopiedId] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [inspectItem, setInspectItem] = useState<CatalogPackage | null>(null)
  const [inspectSuite, setInspectSuite] = useState<SuiteDefinition | null>(null)
  const [inspectLoading, setInspectLoading] = useState(false)
  const [inspectError, setInspectError] = useState('')
  const [pendingFavoriteIds, setPendingFavoriteIds] = useState<Set<string>>(new Set())
  const deferredQuery = useDeferredValue(query)
  const session = getSession()

  const openInspect = (item: CatalogPackage) => {
    setInspectItem(item)
    setInspectSuite(null)
    setInspectError('')
    setInspectLoading(true)
    void getSuite(item.id)
      .then((s) => { setInspectSuite(s); setInspectLoading(false) })
      .catch((e) => { setInspectError(e instanceof Error ? e.message : 'Could not load package.'); setInspectLoading(false) })
  }

  const closeInspect = () => {
    setInspectItem(null)
    setInspectSuite(null)
    setInspectError('')
  }

  useEffect(() => {
    let active = true
    setLoading(true)
    void listCatalogPackages()
      .then((pkgs) => { if (active) { setPackages(pkgs); setLoading(false) } })
      .catch((err) => { if (active) { setError(err instanceof Error ? err.message : 'Could not load catalog.'); setLoading(false) } })
    return () => { active = false }
  }, [])

  const providers = useMemo(
    () => ['All', ...Array.from(new Set(packages.filter((p) => p.kind === tab).map((p) => p.provider)))],
    [packages, tab],
  )

  useEffect(() => {
    if (!providers.includes(provider)) setProvider('All')
  }, [provider, providers])

  const visibleItems = useMemo(() => {
    const filtered = packages.filter((item) => {
      if (item.kind !== tab) return false
      if (provider !== 'All' && item.provider !== provider) return false
      if (activeCapabilities.size > 0) {
        for (const capability of activeCapabilities) {
          if (!matchesCapability(item, capability)) {
            return false
          }
        }
      }
      const hay = [item.title, item.repository, item.owner, item.description, item.modules.join(' '), item.tags.join(' ')].join(' ').toLowerCase()
      return hay.includes(deferredQuery.trim().toLowerCase())
    })
    return [...filtered].sort((a, b) => {
      if (sort === 'starred') {
        const left = a.starred ? 1 : 0
        const right = b.starred ? 1 : 0
        if (left !== right) {
          return right - left
        }
        return a.title.localeCompare(b.title)
      }
      if (sort === 'title') return a.title.localeCompare(b.title)
      return b.version.localeCompare(a.version)
    })
  }, [activeCapabilities, deferredQuery, packages, provider, tab, sort])

  const availableCapabilities = useMemo(() => {
    return CAPABILITY_FILTERS
      .map((capability) => ({
        ...capability,
        count: packages.filter((item) => item.kind === tab && matchesCapability(item, capability.id)).length,
      }))
      .filter((capability) => capability.count > 0)
  }, [packages, tab])

  const toggleFavorite = (packageId: string) => {
    const current = packages.find((item) => item.id === packageId)
    if (!current || pendingFavoriteIds.has(packageId)) {
      return
    }

    const nextStarred = !current.starred
    setError('')
    setPendingFavoriteIds((previous) => new Set(previous).add(packageId))
    setPackages((previous) =>
      previous.map((item) => (item.id === packageId ? { ...item, starred: nextStarred } : item)),
    )

    void (nextStarred ? addCatalogFavorite(packageId) : removeCatalogFavorite(packageId))
      .catch((err) => {
        setPackages((previous) =>
          previous.map((item) => (item.id === packageId ? { ...item, starred: current.starred } : item)),
        )
        setError(err instanceof Error ? err.message : 'Could not update the saved star.')
      })
      .finally(() => {
        setPendingFavoriteIds((previous) => {
          const next = new Set(previous)
          next.delete(packageId)
          return next
        })
      })
  }

  const toggleCapability = (capabilityId: string) => {
    setActiveCapabilities((previous) => {
      const next = new Set(previous)
      if (next.has(capabilityId)) {
        next.delete(capabilityId)
      } else {
        next.add(capabilityId)
      }
      return next
    })
  }

  const copyCommand = async (id: string, value: string) => {
    await navigator.clipboard.writeText(value)
    setCopiedId(id)
    window.setTimeout(() => setCopiedId(''), 1800)
  }

  const clearFilters = () => {
    setProvider('All')
    setActiveCapabilities(new Set())
    setQuery('')
    searchRef.current?.focus()
  }

  const hasActiveFilters = provider !== 'All' || activeCapabilities.size > 0 || query.trim() !== ''
  const activeInspectItem = useMemo(() => {
    if (!inspectItem) {
      return null
    }
    return packages.find((item) => item.id === inspectItem.id) ?? inspectItem
  }, [inspectItem, packages])

  return (
    <AppShell
      section='Catalog'
      title='The Catalog'
      description='Discover official OCI suites and the BabelSuite standard library.'
      actions={(
        <>
          {session?.user.isAdmin && (
            <button type='button' className='catalog-action' onClick={() => navigate('/settings/registries')}>
              <FaGear />
              <span>Registry Settings</span>
            </button>
          )}
        </>
      )}
    >
    <div className='catalog-page'>
      <div className='catalog-body'>
        {/* Mobile filter toggle */}
        <button
          type='button'
          className='catalog-filter-toggle'
          onClick={() => setSidebarOpen((o) => !o)}
          aria-expanded={sidebarOpen}
        >
          <FaMagnifyingGlass />
          <span>Filters</span>
          {hasActiveFilters && <span className='catalog-filter-toggle__dot' />}
        </button>

        {sidebarOpen && <div className='catalog-overlay' onClick={() => setSidebarOpen(false)} />}

        {/* ── Sidebar ── */}
        <aside className={`catalog-sidebar${sidebarOpen ? ' catalog-sidebar--open' : ''}`}>
          <div className='catalog-sidebar__head'>
            <span>Filters</span>
            {hasActiveFilters && (
              <button type='button' className='catalog-clear-btn' onClick={clearFilters}>
                <FaXmark /> Clear all
              </button>
            )}
          </div>

          {/* Kind */}
          <div className='catalog-section'>
            <p className='catalog-section__label'>Kind</p>
            <div className='catalog-kind-list'>
              <button
                type='button'
                className={`catalog-kind-item${tab === 'suite' ? ' catalog-kind-item--active' : ''}`}
                onClick={() => { setTab('suite'); setSidebarOpen(false) }}
              >
                <FaBoxArchive className='catalog-kind-item__icon' />
                <span>Suites</span>
                <span className='catalog-kind-item__count'>{packages.filter((p) => p.kind === 'suite').length}</span>
              </button>
              <button
                type='button'
                className={`catalog-kind-item${tab === 'stdlib' ? ' catalog-kind-item--active' : ''}`}
                onClick={() => { setTab('stdlib'); setSidebarOpen(false) }}
              >
                <FaCubes className='catalog-kind-item__icon' />
                <span>Stdlib</span>
                <span className='catalog-kind-item__count'>{packages.filter((p) => p.kind === 'stdlib').length}</span>
              </button>
            </div>
          </div>

          {/* Registry source */}
          {providers.length > 1 && (
            <div className='catalog-section'>
              <p className='catalog-section__label'>Registry Source</p>
              <div className='catalog-filter-list'>
                {providers.map((p) => {
                  const count = p === 'All'
                    ? packages.filter((pkg) => pkg.kind === tab).length
                    : packages.filter((pkg) => pkg.kind === tab && pkg.provider === p).length
                  return (
                    <button
                      key={p}
                      type='button'
                      className={`catalog-filter-item${provider === p ? ' catalog-filter-item--active' : ''}`}
                      onClick={() => { setProvider(p); setSidebarOpen(false) }}
                    >
                      <span>{p}</span>
                      <span className='catalog-filter-item__count'>{count}</span>
                    </button>
                  )
                })}
              </div>
            </div>
          )}

          {availableCapabilities.length > 0 && (
            <div className='catalog-section'>
              <p className='catalog-section__label'>Capabilities</p>
              <div className='catalog-capability-list'>
                {availableCapabilities.map((capability) => (
                  <button
                    key={capability.id}
                    type='button'
                    className={`catalog-capability${activeCapabilities.has(capability.id) ? ' catalog-capability--active' : ''}`}
                    onClick={() => toggleCapability(capability.id)}
                  >
                    <span>{capability.label}</span>
                    <span className='catalog-capability__count'>{capability.count}</span>
                  </button>
                ))}
              </div>
            </div>
          )}

        </aside>

        {/* ── Results ── */}
        <div className='catalog-results'>
          {/* Search + toolbar */}
          <div className='catalog-toolbar'>
            <label className='catalog-search'>
              <FaMagnifyingGlass className='catalog-search__icon' />
              <input
                ref={searchRef}
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder={tab === 'suite' ? 'Search suites, teams, registry paths…' : 'Search native extensions and contracts…'}
              />
              {query && (
                <button type='button' className='catalog-search__clear' onClick={() => setQuery('')} aria-label='Clear search'>
                  <FaXmark />
                </button>
              )}
            </label>

            <div className='catalog-toolbar__right'>
              <span className='catalog-count'>
                <strong>{visibleItems.length}</strong>
                {' '}{tab === 'suite' ? 'suites' : 'modules'}
              </span>
              <select
                className='catalog-sort'
                value={sort}
                onChange={(e) => setSort(e.target.value as SortKey)}
                aria-label='Sort packages'
              >
                <option value='starred'>Starred first</option>
                <option value='title'>Name A–Z</option>
                <option value='version'>Version</option>
              </select>
            </div>
          </div>

          {/* Active filter chips */}
          {hasActiveFilters && (
            <div className='catalog-active-filters'>
              {query && (
                <span className='catalog-filter-chip'>
                  "{query}"
                  <button type='button' onClick={() => setQuery('')} aria-label='Remove query filter'><FaXmark /></button>
                </span>
              )}
              {provider !== 'All' && (
                <span className='catalog-filter-chip'>
                  {provider}
                  <button type='button' onClick={() => setProvider('All')} aria-label={`Remove ${provider} filter`}><FaXmark /></button>
                </span>
              )}
              {[...activeCapabilities].map((capabilityId) => {
                const capability = CAPABILITY_FILTERS.find((candidate) => candidate.id === capabilityId)
                if (!capability) {
                  return null
                }
                return (
                  <span key={capability.id} className='catalog-filter-chip'>
                    {capability.label}
                    <button type='button' onClick={() => toggleCapability(capability.id)} aria-label={`Remove ${capability.label} filter`}><FaXmark /></button>
                  </span>
                )
              })}
            </div>
          )}

          {error && <div className='catalog-alert catalog-alert--error'>{error}</div>}

          {/* Loading skeletons */}
          {loading && (
            <div className='catalog-list'>
              {[1, 2, 3, 4].map((n) => (
                <div key={n} className='catalog-card catalog-card--skeleton'>
                  <div className='catalog-skeleton__logo' />
                  <div className='catalog-skeleton__body'>
                    <div className='catalog-skeleton__line catalog-skeleton__line--lg' />
                    <div className='catalog-skeleton__line' />
                    <div className='catalog-skeleton__line catalog-skeleton__line--sm' />
                  </div>
                </div>
              ))}
            </div>
          )}

          {/* Package list */}
          {!loading && (
            <div className='catalog-list'>
              {visibleItems.map((item) => (
                <PackageCard
                  key={item.id}
                  item={item}
                  starred={item.starred}
                  copiedId={copiedId}
                  favoriteBusy={pendingFavoriteIds.has(item.id)}
                  onInspect={() => openInspect(item)}
                  onCopyRun={() => void copyCommand(item.id + '-run', item.pullCommand)}
                  onCopyFork={() => void copyCommand(item.id + '-fork', item.forkCommand)}
                  onToggleFavorite={() => toggleFavorite(item.id)}
                />
              ))}

              {visibleItems.length === 0 && (
                <div className='catalog-empty'>
                  <div className='catalog-empty__icon'><FaMagnifyingGlass /></div>
                  <h3>No packages found</h3>
                  <p>Try adjusting your search terms or filters.</p>
                  {hasActiveFilters && (
                    <button type='button' className='catalog-empty__reset' onClick={clearFilters}>
                      Clear all filters
                    </button>
                  )}
                </div>
              )}
            </div>
          )}
        </div>
      </div>
      {copiedId && (
        <div className='catalog-toast'>
          <FaCircleCheck /> Copied to clipboard
        </div>
      )}
    </div>

    {activeInspectItem && (
      <CatalogInspectModal
        item={activeInspectItem}
        suite={inspectSuite}
        starred={activeInspectItem.starred}
        loading={inspectLoading}
        error={inspectError}
        favoriteBusy={pendingFavoriteIds.has(activeInspectItem.id)}
        onClose={closeInspect}
        onToggleFavorite={() => toggleFavorite(activeInspectItem.id)}
      />
    )}
    </AppShell>
  )
}


function matchesCapability(item: CatalogPackage, capabilityId: string) {
  const capability = CAPABILITY_FILTERS.find((candidate) => candidate.id === capabilityId)
  if (!capability) {
    return false
  }
  const haystack = [item.title, item.repository, item.description, item.modules.join(' ')].join(' ').toLowerCase()
  return capability.terms.some((term) => haystack.includes(term))
}
