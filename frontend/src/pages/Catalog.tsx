import { useDeferredValue, useEffect, useMemo, useRef, useState } from 'react'
import { FaGear, FaMagnifyingGlass } from 'react-icons/fa6'
import { useNavigate } from 'react-router-dom'
import AppShell from '../components/AppShell'
import { CatalogInspectModal } from '../components/CatalogInspectModal'
import { CatalogCopyToast, CatalogResults } from '../components/CatalogResults'
import { CatalogSidebar } from '../components/CatalogSidebar'
import { useClipboardFeedback } from '../hooks/useClipboardFeedback'
import {
  addCatalogFavorite,
  getSession,
  getSuite,
  listCatalogPackages,
  removeCatalogFavorite,
  type CatalogKind,
  type CatalogPackage,
  type SuiteDefinition,
} from '../lib/api'
import {
  availableCatalogCapabilities,
  catalogKindCounts,
  catalogProviders,
  CATALOG_ALL_PROVIDER,
  filterCatalogPackages,
  type CatalogSortKey,
} from '../lib/catalog'
import './Catalog.css'

export default function Catalog() {
  const navigate = useNavigate()
  const searchRef = useRef<HTMLInputElement>(null)
  const [tab, setTab] = useState<CatalogKind>('suite')
  const [packages, setPackages] = useState<CatalogPackage[]>([])
  const [query, setQuery] = useState('')
  const [provider, setProvider] = useState(CATALOG_ALL_PROVIDER)
  const [activeCapabilities, setActiveCapabilities] = useState<Set<string>>(new Set())
  const [sort, setSort] = useState<CatalogSortKey>('starred')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [inspectItem, setInspectItem] = useState<CatalogPackage | null>(null)
  const [inspectSuite, setInspectSuite] = useState<SuiteDefinition | null>(null)
  const [inspectLoading, setInspectLoading] = useState(false)
  const [inspectError, setInspectError] = useState('')
  const [pendingFavoriteIds, setPendingFavoriteIds] = useState<Set<string>>(new Set())
  const { copiedId, copyToClipboard } = useClipboardFeedback()
  const deferredQuery = useDeferredValue(query)
  const session = getSession()

  useEffect(() => {
    let active = true
    setLoading(true)
    void listCatalogPackages()
      .then((nextPackages) => {
        if (!active) {
          return
        }
        setPackages(nextPackages)
        setLoading(false)
      })
      .catch((reason) => {
        if (!active) {
          return
        }
        setError(reason instanceof Error ? reason.message : 'Could not load catalog.')
        setLoading(false)
      })
    return () => {
      active = false
    }
  }, [])

  const providers = useMemo(() => catalogProviders(packages, tab), [packages, tab])
  const kindCounts = useMemo(() => catalogKindCounts(packages), [packages])
  const visibleItems = useMemo(
    () => filterCatalogPackages({
      packages,
      kind: tab,
      provider,
      capabilities: activeCapabilities,
      query: deferredQuery,
      sort,
    }),
    [activeCapabilities, deferredQuery, packages, provider, tab, sort],
  )
  const availableCapabilities = useMemo(
    () => availableCatalogCapabilities(packages, tab),
    [packages, tab],
  )
  const activeInspectItem = useMemo(() => {
    if (!inspectItem) {
      return null
    }
    return packages.find((item) => item.id === inspectItem.id) ?? inspectItem
  }, [inspectItem, packages])

  useEffect(() => {
    if (!providers.includes(provider)) {
      setProvider(CATALOG_ALL_PROVIDER)
    }
  }, [provider, providers])

  const openInspect = (item: CatalogPackage) => {
    setInspectItem(item)
    setInspectSuite(null)
    setInspectError('')
    setInspectLoading(true)
    void getSuite(item.id)
      .then((suite) => {
        setInspectSuite(suite)
        setInspectLoading(false)
      })
      .catch((reason) => {
        setInspectError(reason instanceof Error ? reason.message : 'Could not load package.')
        setInspectLoading(false)
      })
  }

  const closeInspect = () => {
    setInspectItem(null)
    setInspectSuite(null)
    setInspectError('')
  }

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
      .catch((reason) => {
        setPackages((previous) =>
          previous.map((item) => (item.id === packageId ? { ...item, starred: current.starred } : item)),
        )
        setError(reason instanceof Error ? reason.message : 'Could not update the saved star.')
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

  const clearFilters = () => {
    setProvider(CATALOG_ALL_PROVIDER)
    setActiveCapabilities(new Set())
    setQuery('')
    searchRef.current?.focus()
  }

  const selectKind = (kind: CatalogKind) => {
    setTab(kind)
    setSidebarOpen(false)
  }

  const selectProvider = (nextProvider: string) => {
    setProvider(nextProvider)
    setSidebarOpen(false)
  }

  const hasActiveFilters = provider !== CATALOG_ALL_PROVIDER || activeCapabilities.size > 0 || query.trim() !== ''

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
          <button
            type='button'
            className='catalog-filter-toggle'
            onClick={() => setSidebarOpen((open) => !open)}
            aria-expanded={sidebarOpen}
          >
            <FaMagnifyingGlass />
            <span>Filters</span>
            {hasActiveFilters && <span className='catalog-filter-toggle__dot' />}
          </button>

          {sidebarOpen && <div className='catalog-overlay' onClick={() => setSidebarOpen(false)} />}

          <CatalogSidebar
            packages={packages}
            tab={tab}
            provider={provider}
            providers={providers}
            activeCapabilities={activeCapabilities}
            availableCapabilities={availableCapabilities}
            kindCounts={kindCounts}
            open={sidebarOpen}
            hasActiveFilters={hasActiveFilters}
            onClearFilters={clearFilters}
            onSelectKind={selectKind}
            onSelectProvider={selectProvider}
            onToggleCapability={toggleCapability}
          />

          <CatalogResults
            searchRef={searchRef}
            tab={tab}
            query={query}
            provider={provider}
            sort={sort}
            items={visibleItems}
            loading={loading}
            error={error}
            activeCapabilities={activeCapabilities}
            copiedId={copiedId}
            pendingFavoriteIds={pendingFavoriteIds}
            hasActiveFilters={hasActiveFilters}
            onChangeQuery={setQuery}
            onClearFilters={clearFilters}
            onSetProvider={setProvider}
            onSetSort={setSort}
            onToggleCapability={toggleCapability}
            onInspect={openInspect}
            onCopyRun={(item) => void copyToClipboard(`${item.id}-run`, item.pullCommand)}
            onCopyFork={(item) => void copyToClipboard(`${item.id}-fork`, item.forkCommand)}
            onToggleFavorite={toggleFavorite}
          />
        </div>

        {copiedId && <CatalogCopyToast />}
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
