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
  FaXmark,
} from 'react-icons/fa6'
import { useNavigate } from 'react-router-dom'
import AppShell from '../components/AppShell'
import { getSession, getSuite, listCatalogPackages, type CatalogPackage, type SuiteDefinition } from '../lib/api'
import './Catalog.css'

type SortKey = 'score' | 'title' | 'version'

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

const CAPABILITY_FALLBACK = ['postgres', 'kafka', 'wiremock', 'vault', 'grpc', 'redis', 'prometheus', 'elasticsearch']

export default function Catalog() {
  const navigate = useNavigate()
  const searchRef = useRef<HTMLInputElement>(null)
  const [tab, setTab] = useState<'suite' | 'stdlib'>('suite')
  const [packages, setPackages] = useState<CatalogPackage[]>([])
  const [query, setQuery] = useState('')
  const [provider, setProvider] = useState('All')
  const [activeCapabilities, setActiveCapabilities] = useState<Set<string>>(new Set())
  const [sort, setSort] = useState<SortKey>('score')
  const [copiedId, setCopiedId] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [inspectItem, setInspectItem] = useState<CatalogPackage | null>(null)
  const [inspectSuite, setInspectSuite] = useState<SuiteDefinition | null>(null)
  const [inspectLoading, setInspectLoading] = useState(false)
  const [inspectError, setInspectError] = useState('')
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
        const haystack = [...item.modules, ...item.tags].join(' ').toLowerCase()
        if (![...activeCapabilities].every((cap) => haystack.includes(cap))) return false
      }
      const hay = [item.title, item.repository, item.owner, item.description, item.modules.join(' ')].join(' ').toLowerCase()
      return hay.includes(deferredQuery.trim().toLowerCase())
    })
    return [...filtered].sort((a, b) => {
      if (sort === 'score') return b.score - a.score
      if (sort === 'title') return a.title.localeCompare(b.title)
      return b.version.localeCompare(a.version)
    })
  }, [deferredQuery, packages, provider, activeCapabilities, tab, sort])

  const capabilities = useMemo(() => {
    const words = new Set<string>()
    packages.forEach((p) => {
      p.modules.forEach((m) => words.add(m.replace(/^@[^/]+\//, '').toLowerCase()))
      p.tags.forEach((t) => words.add(t.toLowerCase()))
    })
    const derived = [...words].filter((w) => w.length > 2).sort()
    return derived.length > 0 ? derived : CAPABILITY_FALLBACK
  }, [packages])

  const toggleCapability = (cap: string) => {
    setActiveCapabilities((prev) => {
      const next = new Set(prev)
      next.has(cap) ? next.delete(cap) : next.add(cap)
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

          {/* Capabilities */}
          {capabilities.length > 0 && (
          <div className='catalog-section'>
            <p className='catalog-section__label'>Capabilities</p>
            <div className='catalog-capability-list'>
              {capabilities.map((cap) => (
                <button
                  key={cap}
                  type='button'
                  className={`catalog-capability${activeCapabilities.has(cap) ? ' catalog-capability--active' : ''}`}
                  onClick={() => toggleCapability(cap)}
                >
                  {activeCapabilities.has(cap) && <FaCircleCheck className='catalog-capability__check' />}
                  {cap}
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
                <option value='score'>Relevance</option>
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
              {[...activeCapabilities].map((cap) => (
                <span key={cap} className='catalog-filter-chip'>
                  {cap}
                  <button type='button' onClick={() => toggleCapability(cap)} aria-label={`Remove ${cap} filter`}><FaXmark /></button>
                </span>
              ))}
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
                  copiedId={copiedId}
                  onInspect={() => openInspect(item)}
                  onCopyRun={() => void copyCommand(item.id + '-run', item.pullCommand)}
                  onCopyFork={() => void copyCommand(item.id + '-fork', item.forkCommand)}
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

    {inspectItem && (
      <InspectModal
        item={inspectItem}
        suite={inspectSuite}
        loading={inspectLoading}
        error={inspectError}
        onClose={closeInspect}
      />
    )}
    </AppShell>
  )
}

/* ── Inspect modal ────────────────────────────────────── */

interface InspectModalProps {
  item: CatalogPackage
  suite: SuiteDefinition | null
  loading: boolean
  error: string
  onClose: () => void
}

function InspectModal({ item, suite, loading, error, onClose }: InspectModalProps) {
  const [selected, setSelected] = useState<string>('suite.star')
  const [copyId, setCopyId] = useState('')

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

  const activeFolder = suite?.folders.find((f) => f.name === selected)

  const treeItems: Array<{ id: string; label: string; icon: ReactNode; kind: 'file' | 'folder' }> = [
    { id: 'suite.star', label: 'suite.star', icon: <FaFile />, kind: 'file' },
    ...(suite?.folders.map((f) => ({
      id: f.name,
      label: `${f.name}/`,
      icon: <FaFolderOpen />,
      kind: 'folder' as const,
    })) ?? []),
  ]

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
          <div className='ci-header__score'><FaRegStar />{item.score.toFixed(1)}</div>
          <button type='button' className='ci-close' onClick={onClose} aria-label='Close'><FaXmark /></button>
        </div>

        {/* Body */}
        <div className='ci-body'>
          {/* Left tree */}
          <aside className='ci-tree'>
            <p className='ci-tree__label'>Package Files</p>
            {loading
              ? [1, 2, 3, 4, 5].map((n) => <div key={n} className='ci-tree__skeleton' />)
              : treeItems.map((t) => (
                  <button
                    key={t.id}
                    type='button'
                    className={`ci-tree__item${selected === t.id ? ' ci-tree__item--active' : ''}`}
                    onClick={() => setSelected(t.id)}
                  >
                    <span className='ci-tree__item-icon'>{t.icon}</span>
                    <span>{t.label}</span>
                  </button>
                ))}
            {suite && (
              <>
                <p className='ci-tree__label' style={{ marginTop: 16 }}>Profiles</p>
                {suite.profiles.map((p) => (
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

            {error && (
              <div className='ci-content__error'>{error}</div>
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

            {!loading && !error && suite && activeFolder && (
              <>
                <div className='ci-content__toolbar'>
                  <span className='ci-content__filename'><FaFolderOpen /> {activeFolder.name}/</span>
                  <span className='ci-folder-role'>{activeFolder.role}</span>
                </div>
                <div className='ci-folder-body'>
                  <p className='ci-folder-desc'>{activeFolder.description}</p>
                  <div className='ci-folder-files'>
                    {activeFolder.files.map((f) => (
                      <div key={f} className='ci-folder-file'>
                        <FaFile className='ci-folder-file__icon' />
                        <span>{f}</span>
                      </div>
                    ))}
                    {activeFolder.files.length === 0 && (
                      <p className='ci-folder-empty'>No files in this folder.</p>
                    )}
                  </div>
                </div>
              </>
            )}

            {!loading && !error && suite && (
              <div className='ci-footer'>
                <button
                  type='button'
                  className={`ci-cmd-btn${copyId === 'pull' ? ' ci-cmd-btn--ok' : ''}`}
                  onClick={() => void copy('pull', suite.pullCommand)}
                >
                  <FaCopy />
                  <span>{copyId === 'pull' ? 'Copied!' : 'Copy pull command'}</span>
                </button>
                <button
                  type='button'
                  className={`ci-cmd-btn ci-cmd-btn--ghost${copyId === 'fork' ? ' ci-cmd-btn--ok' : ''}`}
                  onClick={() => void copy('fork', suite.forkCommand)}
                >
                  <FaDownload />
                  <span>{copyId === 'fork' ? 'Copied!' : 'Fork'}</span>
                </button>
                {item.modules.length > 0 && (
                  <div className='ci-modules'>
                    {item.modules.map((m) => (
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
  const pat = /"[^"]*"|\b(load|container|mock|script|scenario)\b|@[a-zA-Z0-9/_-]+/g
  let cur = 0
  for (const m of code.matchAll(pat)) {
    const v = m[0]; const s = m.index ?? 0
    if (s > cur) out.push(code.slice(cur, s))
    const cls = v.startsWith('"') ? 'ci-tok ci-tok--str' : v.startsWith('@') ? 'ci-tok ci-tok--mod' : 'ci-tok ci-tok--kw'
    out.push(<span key={`${s}-${v}`} className={cls}>{v}</span>)
    cur = s + v.length
  }
  if (cur < code.length) out.push(code.slice(cur))
  if (comment) out.push(<span key='cmt' className='ci-tok ci-tok--cmt'>{comment}</span>)
  return out
}

/* ── Package Card component ───────────────────────────── */

interface PackageCardProps {
  item: CatalogPackage
  copiedId: string
  onInspect: () => void
  onCopyRun: () => void
  onCopyFork: () => void
}

function PackageCard({ item, copiedId, onInspect, onCopyRun, onCopyFork }: PackageCardProps) {
  const runCopied = copiedId === item.id + '-run'
  const forkCopied = copiedId === item.id + '-fork'

  return (
    <article className='catalog-card'>
      {/* Header row: logo + headline + score */}
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

        <div className='catalog-card__score'>
          <FaRegStar />
          <span>{item.score.toFixed(1)}</span>
        </div>
      </div>

      {/* Description */}
      <p className='catalog-card__description'>{item.description}</p>

      {/* Module + tag pills */}
      {(item.modules.length > 0 || item.tags.length > 0) && (
        <div className='catalog-card__pills'>
          {item.modules.map((m) => (
            <span key={m} className='catalog-pill catalog-pill--module'>
              <FaCubes /> {m.replace('@babelsuite/', '')}
            </span>
          ))}
          {item.tags.map((tag) => (
            <span key={tag} className='catalog-pill catalog-pill--tag'>{tag}</span>
          ))}
        </div>
      )}

      {/* Footer: source + actions */}
      <div className='catalog-card__footer'>
        <span className='catalog-card__provider'>
          <FaServer /> {item.provider}
        </span>

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
