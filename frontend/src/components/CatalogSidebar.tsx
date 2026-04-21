import { FaBoxArchive, FaCubes, FaXmark } from 'react-icons/fa6'
import type { ReactNode } from 'react'
import type { CatalogKind, CatalogPackage } from '../lib/api'
import {
  catalogProviderCount,
  type CatalogCapabilityId,
} from '../lib/catalog'
import './CatalogSidebar.css'

interface CatalogSidebarProps {
  packages: CatalogPackage[]
  tab: CatalogKind
  provider: string
  providers: string[]
  activeCapabilities: ReadonlySet<string>
  availableCapabilities: Array<{ id: CatalogCapabilityId; label: string; count: number }>
  kindCounts: Record<CatalogKind, number>
  open: boolean
  hasActiveFilters: boolean
  onClearFilters: () => void
  onSelectKind: (kind: CatalogKind) => void
  onSelectProvider: (provider: string) => void
  onToggleCapability: (capabilityId: string) => void
}

export function CatalogSidebar({
  packages,
  tab,
  provider,
  providers,
  activeCapabilities,
  availableCapabilities,
  kindCounts,
  open,
  hasActiveFilters,
  onClearFilters,
  onSelectKind,
  onSelectProvider,
  onToggleCapability,
}: CatalogSidebarProps) {
  return (
    <aside className={`catalog-sidebar${open ? ' catalog-sidebar--open' : ''}`}>
      <div className='catalog-sidebar__head'>
        <span>Filters</span>
        {hasActiveFilters && (
          <button type='button' className='catalog-clear-btn' onClick={onClearFilters}>
            <FaXmark /> Clear all
          </button>
        )}
      </div>

      <div className='catalog-section'>
        <p className='catalog-section__label'>Kind</p>
        <div className='catalog-kind-list'>
          <CatalogKindButton
            active={tab === 'suite'}
            count={kindCounts.suite}
            icon={<FaBoxArchive className='catalog-kind-item__icon' />}
            label='Suites'
            onClick={() => onSelectKind('suite')}
          />
          <CatalogKindButton
            active={tab === 'stdlib'}
            count={kindCounts.stdlib}
            icon={<FaCubes className='catalog-kind-item__icon' />}
            label='Stdlib'
            onClick={() => onSelectKind('stdlib')}
          />
        </div>
      </div>

      {providers.length > 1 && (
        <div className='catalog-section'>
          <p className='catalog-section__label'>Registry Source</p>
          <div className='catalog-filter-list'>
            {providers.map((candidate) => (
              <button
                key={candidate}
                type='button'
                className={`catalog-filter-item${provider === candidate ? ' catalog-filter-item--active' : ''}`}
                onClick={() => onSelectProvider(candidate)}
              >
                <span>{candidate}</span>
                <span className='catalog-filter-item__count'>
                  {catalogProviderCount(packages, tab, candidate)}
                </span>
              </button>
            ))}
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
                onClick={() => onToggleCapability(capability.id)}
              >
                <span>{capability.label}</span>
                <span className='catalog-capability__count'>{capability.count}</span>
              </button>
            ))}
          </div>
        </div>
      )}
    </aside>
  )
}

interface CatalogKindButtonProps {
  active: boolean
  count: number
  icon: ReactNode
  label: string
  onClick: () => void
}

function CatalogKindButton({ active, count, icon, label, onClick }: CatalogKindButtonProps) {
  return (
    <button
      type='button'
      className={`catalog-kind-item${active ? ' catalog-kind-item--active' : ''}`}
      onClick={onClick}
    >
      {icon}
      <span>{label}</span>
      <span className='catalog-kind-item__count'>{count}</span>
    </button>
  )
}
