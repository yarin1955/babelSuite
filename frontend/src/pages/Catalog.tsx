import type { ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { useDeferredValue, useEffect, useMemo, useRef, useState } from 'react'
import {
  FaBoxArchive,
  FaCircleCheck,
  FaCopy,
  FaCubes,
  FaDiagramProject,
  FaDownload,
  FaFile,
  FaFolderOpen,
  FaGear,
  FaMagnifyingGlass,
  FaRegStar,
  FaServer,
  FaShieldHalved,
  FaStar,
  FaXmark,
} from 'react-icons/fa6'
import { useNavigate } from 'react-router-dom'
import AppShell from '../components/AppShell'
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

const LOGO_GRADIENTS = [
  'linear-gradient(135deg, #173b5b 0%, #1f7ea8 100%)',
  'linear-gradient(135deg, #1a3a5c 0%, #0DADEA 100%)',
  'linear-gradient(135deg, #2c1654 0%, #7e3fb3 100%)',
  'linear-gradient(135deg, #0f3d2b 0%, #18BE94 100%)',
  'linear-gradient(135deg, #3d1f0a 0%, #f77530 100%)',
  'linear-gradient(135deg, #1a0f3d 0%, #5b4ee8 100%)',
  'linear-gradient(135deg, #3d0f1a 0%, #e84e6e 100%)',
  'linear-gradient(135deg, #1a3d0f 0%, #5cb84e 100%)',
]

function logoGradient(seed: string): string {
  let h = 0
  for (let i = 0; i < seed.length; i++) h = (Math.imul(31, h) + seed.charCodeAt(i)) | 0
  return LOGO_GRADIENTS[Math.abs(h) % LOGO_GRADIENTS.length]
}

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
      <InspectModal
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

/* ── Inspect modal ────────────────────────────────────── */

interface InspectModalProps {
  item: CatalogPackage
  suite: SuiteDefinition | null
  starred: boolean
  loading: boolean
  error: string
  favoriteBusy: boolean
  onClose: () => void
  onToggleFavorite: () => void
}

function InspectModal({ item, suite, starred, loading, error, favoriteBusy, onClose, onToggleFavorite }: InspectModalProps) {
  const [selected, setSelected] = useState<string>('suite.star')
  const [copyId, setCopyId] = useState('')
  const fallbackSourceFile = useMemo(() => buildRegistryPreviewFile(item, error), [item, error])
  const suiteSourceFiles = suite?.sourceFiles ?? []
  const suiteFolders = suite?.folders ?? []
  const rootSourceFiles = useMemo(
    () => suiteSourceFiles.filter((file) => !file.path.includes('/')),
    [suiteSourceFiles],
  )
  const suiteProfiles = suite?.profiles ?? []
  const sourceFileByPath = useMemo(
    () => new Map(suiteSourceFiles.map((file) => [file.path, file])),
    [suiteSourceFiles],
  )

  const copy = async (id: string, value: string) => {
    await navigator.clipboard.writeText(value)
    setCopyId(id)
    window.setTimeout(() => setCopyId(''), 1600)
  }

  // close on Escape
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  useEffect(() => {
    setSelected(suite ? 'suite.star' : fallbackSourceFile.path)
  }, [fallbackSourceFile.path, suite, item.id])

  const activeSourceFile = sourceFileByPath.get(selected)
  const visibleSourceFile = activeSourceFile ?? (!suite ? fallbackSourceFile : null)
  const visibleModules = suite?.modules ?? item.modules
  const pullCommand = suite?.pullCommand ?? item.pullCommand
  const forkCommand = suite?.forkCommand ?? item.forkCommand
  const showRichSourceUnavailable = !loading && Boolean(suite) && selected !== 'suite.star' && !activeSourceFile

  return createPortal(
    <div className='ci-backdrop' onClick={onClose}>
      <div className='ci-modal' onClick={(e) => e.stopPropagation()}>

        {/* Header */}
        <div className='ci-header'>
          <div className='ci-header__logo' style={{ background: logoGradient(item.id) }}>
            {item.title.slice(0, 2).toUpperCase()}
          </div>
          <div className='ci-header__meta'>
            <div className='ci-header__title-row'>
              <h2 className='ci-header__title'>{item.title}</h2>
              {item.status === 'Official' && (
                <span className='ci-badge ci-badge--official'><FaShieldHalved /> Official</span>
              )}
              {item.status === 'Verified' && (
                <span className='ci-badge ci-badge--verified'><FaCircleCheck /> Verified</span>
              )}
            </div>
            <p className='ci-header__origin'>
              {item.owner} · {item.repository} · <strong>{item.version}</strong>
            </p>
          </div>
          <button
            type='button'
            className={`ci-header__star${starred ? ' ci-header__star--active' : ''}`}
            aria-pressed={starred}
            aria-label={starred ? `Unstar ${item.title}` : `Star ${item.title}`}
            title={starred ? 'Unstar' : 'Star'}
            disabled={favoriteBusy}
            onClick={onToggleFavorite}
          >
            {starred ? <FaStar /> : <FaRegStar />}
          </button>
          <button type='button' className='ci-close' onClick={onClose} aria-label='Close'><FaXmark /></button>
        </div>

        {/* Body */}
        <div className='ci-body'>
          {/* Left tree */}
          <aside className='ci-tree'>
            <p className='ci-tree__label'>Package Files</p>
            {loading
              ? [1, 2, 3, 4, 5].map((n) => <div key={n} className='ci-tree__skeleton' />)
              : (
                  <>
                    {suite ? (
                      <button
                        type='button'
                        className={`ci-tree__item${selected === 'suite.star' ? ' ci-tree__item--active' : ''}`}
                        onClick={() => setSelected('suite.star')}
                      >
                        <span className='ci-tree__item-icon'><FaFile /></span>
                        <span>suite.star</span>
                      </button>
                    ) : (
                      <button
                        type='button'
                        className={`ci-tree__item${selected === fallbackSourceFile.path ? ' ci-tree__item--active' : ''}`}
                        onClick={() => setSelected(fallbackSourceFile.path)}
                      >
                        <span className='ci-tree__item-icon'><FaFile /></span>
                        <span>{fallbackSourceFile.path}</span>
                      </button>
                    )}
                    {rootSourceFiles.map((file) => (
                      <button
                        key={file.path}
                        type='button'
                        className={`ci-tree__item${selected === file.path ? ' ci-tree__item--active' : ''}`}
                        onClick={() => setSelected(file.path)}
                      >
                        <span className='ci-tree__item-icon'><FaFile /></span>
                        <span>{file.path}</span>
                      </button>
                    ))}
                    {suiteFolders.map((folder) => (
                      <div key={folder.name} className='ci-tree__group'>
                        <div className='ci-tree__folder'>
                          <span className='ci-tree__item-icon'><FaFolderOpen /></span>
                          <span>{folder.name}/</span>
                        </div>
                        {folder.files.map((filePath) => {
                          const sourcePath = `${folder.name}/${filePath}`
                          const file = sourceFileByPath.get(sourcePath)
                          return (
                            <button
                              key={sourcePath}
                              type='button'
                              className={`ci-tree__item ci-tree__item--child${selected === sourcePath ? ' ci-tree__item--active' : ''}`}
                              onClick={() => setSelected(sourcePath)}
                              disabled={!file}
                            >
                              <span className='ci-tree__item-icon'><FaFile /></span>
                              <span>{filePath}</span>
                            </button>
                          )
                        })}
                      </div>
                    ))}
                  </>
                )}
            {suiteProfiles.length > 0 && (
              <>
                <p className='ci-tree__label' style={{ marginTop: 16 }}>Profiles</p>
                {suiteProfiles.map((p) => (
                  <div key={p.fileName} className='ci-tree__profile'>
                    <span>{p.label}</span>
                    {p.default && <span className='ci-tree__default'>default</span>}
                  </div>
                ))}
              </>
            )}
          </aside>

          {/* Right content */}
          <div className='ci-content'>
            {loading && (
              <div className='ci-content__loading'>
                <div className='ci-spinner' />
                <p>Loading package…</p>
              </div>
            )}

            {!loading && !error && suite && selected === 'suite.star' && (
              <>
                <div className='ci-content__toolbar'>
                  <span className='ci-content__filename'><FaFile /> suite.star</span>
                  <button
                    type='button'
                    className={`ci-copy-btn${copyId === 'star' ? ' ci-copy-btn--ok' : ''}`}
                    onClick={() => void copy('star', suite.suiteStar)}
                  >
                    <FaCopy />
                    <span>{copyId === 'star' ? 'Copied!' : 'Copy'}</span>
                  </button>
                </div>
                <div className='ci-code'>
                  {suite.suiteStar.split('\n').map((line, i) => (
                    <div key={i} className='ci-code__line'>
                      <span className='ci-code__num'>{String(i + 1).padStart(3, ' ')}</span>
                      <code className='ci-code__text'>{renderStarLine(line)}</code>
                    </div>
                  ))}
                </div>
              </>
            )}

            {!loading && visibleSourceFile && selected !== 'suite.star' && (
              <>
                <div className='ci-content__toolbar'>
                  <span className='ci-content__filename'><FaFile /> {visibleSourceFile.path}</span>
                  <span className='ci-folder-role'>{visibleSourceFile.language}</span>
                  <button
                    type='button'
                    className={`ci-copy-btn${copyId === visibleSourceFile.path ? ' ci-copy-btn--ok' : ''}`}
                    onClick={() => void copy(visibleSourceFile.path, visibleSourceFile.content)}
                  >
                    <FaCopy />
                    <span>{copyId === visibleSourceFile.path ? 'Copied!' : 'Copy'}</span>
                  </button>
                </div>
                {error && (
                  <div className='ci-content__notice'>
                    {error}
                  </div>
                )}
                <div className='ci-code'>
                  {visibleSourceFile.content.split('\n').map((line, i) => (
                    <div key={`${visibleSourceFile.path}-${i + 1}`} className='ci-code__line'>
                      <span className='ci-code__num'>{String(i + 1).padStart(3, ' ')}</span>
                      <code className='ci-code__text'>{renderSourceLine(line, visibleSourceFile.language)}</code>
                    </div>
                  ))}
                </div>
              </>
            )}

            {showRichSourceUnavailable && (
              <div className='ci-content__empty'>
                Source content for this file is not available yet. The suite published the folder entry, but not the file preview.
              </div>
            )}

            {!loading && (
              <div className='ci-footer'>
                <button
                  type='button'
                  className={`ci-cmd-btn${copyId === 'pull' ? ' ci-cmd-btn--ok' : ''}`}
                  onClick={() => void copy('pull', pullCommand)}
                >
                  <FaCopy />
                  <span>{copyId === 'pull' ? 'Copied!' : 'Copy pull command'}</span>
                </button>
                <button
                  type='button'
                  className={`ci-cmd-btn ci-cmd-btn--ghost${copyId === 'fork' ? ' ci-cmd-btn--ok' : ''}`}
                  onClick={() => void copy('fork', forkCommand)}
                >
                  <FaDownload />
                  <span>{copyId === 'fork' ? 'Copied!' : 'Fork'}</span>
                </button>
                {visibleModules.length > 0 && (
                  <div className='ci-modules'>
                    {visibleModules.map((m) => (
                      <span key={m} className='ci-module-pill'>
                        <FaCubes />{m.replace('@babelsuite/', '')}
                      </span>
                    ))}
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>,
    document.body,
  )
}

function renderStarLine(line: string): ReactNode[] {
  const ci = line.indexOf('#')
  const code = ci >= 0 ? line.slice(0, ci) : line
  const comment = ci >= 0 ? line.slice(ci) : ''
  const out: ReactNode[] = []
  const pat = /"[^"]*"|\b(load|service|task|test|traffic|suite|container|mock|script|scenario)\b|@[a-zA-Z0-9/_-]+/g
  let cur = 0
  for (const m of code.matchAll(pat)) {
    const v = m[0]
    const s = m.index ?? 0
    if (s > cur) out.push(code.slice(cur, s))
    const cls = v.startsWith('"') ? 'ci-tok ci-tok--str' : v.startsWith('@') ? 'ci-tok ci-tok--mod' : 'ci-tok ci-tok--kw'
    out.push(<span key={`${s}-${v}`} className={cls}>{v}</span>)
    cur = s + v.length
  }
  if (cur < code.length) out.push(code.slice(cur))
  if (comment) out.push(<span key='cmt' className='ci-tok ci-tok--cmt'>{comment}</span>)
  return out
}

function renderSourceLine(line: string, language: string): ReactNode[] {
  const trimmedLanguage = language.trim().toLowerCase()
  if (trimmedLanguage === 'yaml' || trimmedLanguage === 'python' || trimmedLanguage === 'bash' || trimmedLanguage === 'rego') {
    const commentIndex = line.indexOf('#')
    const code = commentIndex >= 0 ? line.slice(0, commentIndex) : line
    const comment = commentIndex >= 0 ? line.slice(commentIndex) : ''
    const fragments = highlightCodeTokens(code)
    if (comment) {
      fragments.push(<span key={`comment-${line}`} className='ci-tok ci-tok--cmt'>{comment}</span>)
    }
    return fragments
  }

  return highlightCodeTokens(line)
}

function highlightCodeTokens(line: string): ReactNode[] {
  const fragments: ReactNode[] = []
  const pattern = /"[^"]*"|'[^']*'|\b(message|service|rpc|package|import|const|let|type|interface|export|default|allow|if|true|false|null)\b|@[a-zA-Z0-9/_-]+/g
  let cursor = 0

  for (const match of line.matchAll(pattern)) {
    const value = match[0]
    const start = match.index ?? 0
    if (start > cursor) {
      fragments.push(line.slice(cursor, start))
    }

    const className = value.startsWith('"') || value.startsWith("'")
      ? 'ci-tok ci-tok--str'
      : value.startsWith('@')
        ? 'ci-tok ci-tok--mod'
        : 'ci-tok ci-tok--kw'
    fragments.push(<span key={`${start}-${value}`} className={className}>{value}</span>)
    cursor = start + value.length
  }

  if (cursor < line.length) {
    fragments.push(line.slice(cursor))
  }

  return fragments
}

function buildRegistryPreviewFile(item: CatalogPackage, error: string): { path: string; language: string; content: string } {
  const modules = item.modules.length > 0 ? item.modules.map((module) => `  - ${module}`) : ['  - none published']
  const tags = item.tags.length > 0 ? item.tags.map((tag) => `  - ${tag}`) : ['  - latest']
  const lines = [
    '# BabelSuite registry preview',
    `title: ${item.title}`,
    `repository: ${item.repository}`,
    `provider: ${item.provider}`,
    `version: ${item.version}`,
    `status: ${item.status}`,
    'modules:',
    ...modules,
    'publishedTags:',
    ...tags,
    'commands:',
    `  pull: ${item.pullCommand}`,
    `  fork: ${item.forkCommand}`,
    'description: |',
    ...item.description.split('\n').map((line) => `  ${line}`),
  ]

  if (error.trim()) {
    lines.push(
      'note: |',
      `  ${error}`,
      '  Rich suite files are not available from the backend for this registry package yet.',
    )
  }

  return {
    path: 'registry-preview.yaml',
    language: 'yaml',
    content: lines.join('\n') + '\n',
  }
}

/* ── Package Card component ───────────────────────────── */

interface PackageCardProps {
  item: CatalogPackage
  starred: boolean
  copiedId: string
  favoriteBusy: boolean
  onInspect: () => void
  onCopyRun: () => void
  onCopyFork: () => void
  onToggleFavorite: () => void
}

function matchesCapability(item: CatalogPackage, capabilityId: string) {
  const capability = CAPABILITY_FILTERS.find((candidate) => candidate.id === capabilityId)
  if (!capability) {
    return false
  }

  const haystack = [item.title, item.repository, item.description, item.modules.join(' ')].join(' ').toLowerCase()
  return capability.terms.some((term) => haystack.includes(term))
}

function PackageCard({ item, starred, copiedId, favoriteBusy, onInspect, onCopyRun, onCopyFork, onToggleFavorite }: PackageCardProps) {
  const runCopied = copiedId === item.id + '-run'
  const forkCopied = copiedId === item.id + '-fork'
  const visibleModules = item.modules.slice(0, 3)
  const hiddenModuleCount = Math.max(item.modules.length - visibleModules.length, 0)
  const publishedVersions = countPublishedVersions(item.tags)
  const versionSummary = publishedVersions > 1 ? `${publishedVersions} versions available` : '1 version available'
  const draggable = item.kind === 'suite'

  return (
    <article
      className='catalog-card'
      draggable={draggable}
      onDragStart={(event) => {
        if (!draggable) {
          return
        }
        const ref = buildSuiteDropRef(item.repository, item.version)
        event.dataTransfer.effectAllowed = 'copy'
        event.dataTransfer.setData(SANDBOX_SUITE_REF_MIME, ref)
        event.dataTransfer.setData('text/plain', ref)
      }}
    >
      {/* Header row: logo + headline + star */}
      <div className='catalog-card__header'>
        <div className='catalog-card__logo' aria-hidden='true' style={{ background: logoGradient(item.id) }}>
          {item.title.slice(0, 2).toUpperCase()}
        </div>

        <div className='catalog-card__headline'>
          <div className='catalog-card__title-row'>
            <h2 className='catalog-card__title'>{item.title}</h2>
            {item.status === 'Official' && (
              <span className='catalog-badge catalog-badge--official'>
                <FaShieldHalved /> Official
              </span>
            )}
            {item.status === 'Verified' && (
              <span className='catalog-badge catalog-badge--verified'>
                <FaCircleCheck /> Verified
              </span>
            )}
          </div>
          <p className='catalog-card__origin'>
            <span>{item.owner}</span>
            <span className='catalog-card__sep' aria-hidden='true'> · </span>
            <span>{item.repository}</span>
            <span className='catalog-card__sep' aria-hidden='true'> · </span>
            <strong className='catalog-card__version'>{item.version}</strong>
          </p>
        </div>

        <button
          type='button'
          className={`catalog-card__star${starred ? ' catalog-card__star--active' : ''}`}
          aria-pressed={starred}
          aria-label={starred ? `Unstar ${item.title}` : `Star ${item.title}`}
          title={starred ? 'Unstar' : 'Star'}
          disabled={favoriteBusy}
          onClick={onToggleFavorite}
        >
          {starred ? <FaStar /> : <FaRegStar />}
        </button>
      </div>

      {/* Description */}
      <p className='catalog-card__description'>{item.description}</p>

      {/* Module + tag pills */}
      {visibleModules.length > 0 && (
        <div className='catalog-card__pills'>
          {visibleModules.map((m) => (
            <span key={m} className='catalog-pill catalog-pill--module'>
              <FaCubes /> {m.replace('@babelsuite/', '')}
            </span>
          ))}
          {hiddenModuleCount > 0 && <span className='catalog-pill catalog-pill--more'>+{hiddenModuleCount} more</span>}
        </div>
      )}

      {/* Footer: source + actions */}
      <div className='catalog-card__footer'>
        <div className='catalog-card__meta'>
          <span className='catalog-card__provider'>
            <FaServer /> {item.provider}
          </span>
          <span className='catalog-card__versions'>{versionSummary}</span>
        </div>

        <div className='catalog-card__actions'>
          {item.kind === 'suite' && (
            <button type='button' className='catalog-btn catalog-btn--primary' onClick={onInspect}>
              <FaDiagramProject />
              <span>Inspect</span>
            </button>
          )}
          <button
            type='button'
            className={`catalog-btn${runCopied ? ' catalog-btn--copied' : ' catalog-btn--secondary'}`}
            onClick={onCopyRun}
          >
            <FaCopy />
            <span>{runCopied ? 'Copied!' : 'Run command'}</span>
          </button>
          <button
            type='button'
            className={`catalog-btn${forkCopied ? ' catalog-btn--copied' : ' catalog-btn--ghost'}`}
            onClick={onCopyFork}
          >
            <FaDownload />
            <span>{forkCopied ? 'Copied!' : 'Fork'}</span>
          </button>
        </div>
      </div>
    </article>
  )
}

function countPublishedVersions(tags: string[]): number {
  const stableTags = new Set(
    tags
      .map((tag) => tag.trim())
      .filter((tag) => tag !== '' && tag.toLowerCase() !== 'latest'),
  )

  return Math.max(stableTags.size, 1)
}
