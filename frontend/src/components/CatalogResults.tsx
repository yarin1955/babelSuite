import type { RefObject } from 'react'
import { FaCircleCheck, FaMagnifyingGlass, FaXmark } from 'react-icons/fa6'
import type { CatalogKind, CatalogPackage } from '../lib/api'
import {
  CATALOG_ALL_PROVIDER,
  CATALOG_CAPABILITY_FILTERS,
  type CatalogSortKey,
} from '../lib/catalog'
import { PackageCard } from './PackageCard'
import './CatalogResults.css'

interface CatalogResultsProps {
  searchRef: RefObject<HTMLInputElement | null>
  tab: CatalogKind
  query: string
  provider: string
  sort: CatalogSortKey
  items: CatalogPackage[]
  loading: boolean
  error: string
  activeCapabilities: ReadonlySet<string>
  copiedId: string
  pendingFavoriteIds: ReadonlySet<string>
  hasActiveFilters: boolean
  onChangeQuery: (query: string) => void
  onClearFilters: () => void
  onSetProvider: (provider: string) => void
  onSetSort: (sort: CatalogSortKey) => void
  onToggleCapability: (capabilityId: string) => void
  onInspect: (item: CatalogPackage) => void
  onCopyRun: (item: CatalogPackage) => void
  onCopyFork: (item: CatalogPackage) => void
  onToggleFavorite: (packageId: string) => void
}

export function CatalogResults({
  searchRef,
  tab,
  query,
  provider,
  sort,
  items,
  loading,
  error,
  activeCapabilities,
  copiedId,
  pendingFavoriteIds,
  hasActiveFilters,
  onChangeQuery,
  onClearFilters,
  onSetProvider,
  onSetSort,
  onToggleCapability,
  onInspect,
  onCopyRun,
  onCopyFork,
  onToggleFavorite,
}: CatalogResultsProps) {
  return (
    <div className='catalog-results'>
      <CatalogToolbar
        searchRef={searchRef}
        tab={tab}
        query={query}
        sort={sort}
        itemCount={items.length}
        onChangeQuery={onChangeQuery}
        onSetSort={onSetSort}
      />

      {hasActiveFilters && (
        <CatalogActiveFilters
          query={query}
          provider={provider}
          activeCapabilities={activeCapabilities}
          onChangeQuery={onChangeQuery}
          onSetProvider={onSetProvider}
          onToggleCapability={onToggleCapability}
        />
      )}

      {error && <div className='catalog-alert catalog-alert--error'>{error}</div>}

      {loading ? (
        <CatalogLoadingList />
      ) : (
        <div className='catalog-list'>
          {items.map((item) => (
            <PackageCard
              key={item.id}
              item={item}
              starred={item.starred}
              copiedId={copiedId}
              favoriteBusy={pendingFavoriteIds.has(item.id)}
              onInspect={() => onInspect(item)}
              onCopyRun={() => onCopyRun(item)}
              onCopyFork={() => onCopyFork(item)}
              onToggleFavorite={() => onToggleFavorite(item.id)}
            />
          ))}

          {items.length === 0 && (
            <CatalogEmptyState hasActiveFilters={hasActiveFilters} onClearFilters={onClearFilters} />
          )}
        </div>
      )}
    </div>
  )
}

interface CatalogToolbarProps {
  searchRef: RefObject<HTMLInputElement | null>
  tab: CatalogKind
  query: string
  sort: CatalogSortKey
  itemCount: number
  onChangeQuery: (query: string) => void
  onSetSort: (sort: CatalogSortKey) => void
}

function CatalogToolbar({
  searchRef,
  tab,
  query,
  sort,
  itemCount,
  onChangeQuery,
  onSetSort,
}: CatalogToolbarProps) {
  return (
    <div className='catalog-toolbar'>
      <label className='catalog-search'>
        <FaMagnifyingGlass className='catalog-search__icon' />
        <input
          ref={searchRef}
          value={query}
          onChange={(event) => onChangeQuery(event.target.value)}
          placeholder={tab === 'suite' ? 'Search suites, teams, registry paths...' : 'Search native extensions and contracts...'}
        />
        {query && (
          <button type='button' className='catalog-search__clear' onClick={() => onChangeQuery('')} aria-label='Clear search'>
            <FaXmark />
          </button>
        )}
      </label>

      <div className='catalog-toolbar__right'>
        <span className='catalog-count'>
          <strong>{itemCount}</strong>
          {' '}{tab === 'suite' ? 'suites' : 'modules'}
        </span>
        <select
          className='catalog-sort'
          value={sort}
          onChange={(event) => onSetSort(event.target.value as CatalogSortKey)}
          aria-label='Sort packages'
        >
          <option value='starred'>Starred first</option>
          <option value='title'>Name A-Z</option>
          <option value='version'>Version</option>
        </select>
      </div>
    </div>
  )
}

interface CatalogActiveFiltersProps {
  query: string
  provider: string
  activeCapabilities: ReadonlySet<string>
  onChangeQuery: (query: string) => void
  onSetProvider: (provider: string) => void
  onToggleCapability: (capabilityId: string) => void
}

function CatalogActiveFilters({
  query,
  provider,
  activeCapabilities,
  onChangeQuery,
  onSetProvider,
  onToggleCapability,
}: CatalogActiveFiltersProps) {
  return (
    <div className='catalog-active-filters'>
      {query && (
        <span className='catalog-filter-chip'>
          "{query}"
          <button type='button' onClick={() => onChangeQuery('')} aria-label='Remove query filter'>
            <FaXmark />
          </button>
        </span>
      )}
      {provider !== CATALOG_ALL_PROVIDER && (
        <span className='catalog-filter-chip'>
          {provider}
          <button type='button' onClick={() => onSetProvider(CATALOG_ALL_PROVIDER)} aria-label={`Remove ${provider} filter`}>
            <FaXmark />
          </button>
        </span>
      )}
      {[...activeCapabilities].map((capabilityId) => {
        const capability = CATALOG_CAPABILITY_FILTERS.find((candidate) => candidate.id === capabilityId)
        if (!capability) {
          return null
        }
        return (
          <span key={capability.id} className='catalog-filter-chip'>
            {capability.label}
            <button type='button' onClick={() => onToggleCapability(capability.id)} aria-label={`Remove ${capability.label} filter`}>
              <FaXmark />
            </button>
          </span>
        )
      })}
    </div>
  )
}

function CatalogLoadingList() {
  return (
    <div className='catalog-list'>
      {[1, 2, 3, 4].map((item) => (
        <div key={item} className='catalog-card catalog-card--skeleton'>
          <div className='catalog-skeleton__logo' />
          <div className='catalog-skeleton__body'>
            <div className='catalog-skeleton__line catalog-skeleton__line--lg' />
            <div className='catalog-skeleton__line' />
            <div className='catalog-skeleton__line catalog-skeleton__line--sm' />
          </div>
        </div>
      ))}
    </div>
  )
}

function CatalogEmptyState({
  hasActiveFilters,
  onClearFilters,
}: {
  hasActiveFilters: boolean
  onClearFilters: () => void
}) {
  return (
    <div className='catalog-empty'>
      <div className='catalog-empty__icon'><FaMagnifyingGlass /></div>
      <h3>No packages found</h3>
      <p>Try adjusting your search terms or filters.</p>
      {hasActiveFilters && (
        <button type='button' className='catalog-empty__reset' onClick={onClearFilters}>
          Clear all filters
        </button>
      )}
    </div>
  )
}

export function CatalogCopyToast() {
  return (
    <div className='catalog-toast'>
      <FaCircleCheck /> Copied to clipboard
    </div>
  )
}
