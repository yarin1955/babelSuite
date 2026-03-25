import { useEffect, useMemo, useRef, useState } from 'react'
import { FaBoxOpen, FaGear, FaPlay } from 'react-icons/fa6'
import { useNavigate } from 'react-router-dom'
import Layout from '../components/Layout'
import Page from '../components/Page'
import styles from './Catalog.module.css'

const API = 'http://localhost:8090'

interface CatalogPackage {
  package_id: string
  org_id: string
  registry_id: string
  registry_kind: string
  name: string
  display_name: string
  description: string
  publisher: string
  image_ref: string
  version: string
  tags: string[]
  profiles?: string[]
  default_profile?: string
  service_count?: number
  mock_count?: number
  test_count?: number
  contract_count?: number
  enabled: boolean
  updated_at: string
}

interface CatalogResponse {
  packages: CatalogPackage[]
  total: number
  page: number
  page_size: number
  total_pages: number
}

const capabilities = [
  { value: '', label: 'All signals' },
  { value: 'mock', label: 'Mock-ready' },
  { value: 'contract', label: 'Contract-first' },
  { value: 'grpc', label: 'gRPC' },
  { value: 'async', label: 'Async' },
]

const authHeaders = () => ({ 'Content-Type': 'application/json', Authorization: `Bearer ${localStorage.getItem('token')}` })
const isAdmin = () => { try { return JSON.parse(localStorage.getItem('user') || '{}').is_admin === true } catch { return false } }

function capsForPackage(pkg: CatalogPackage) {
  const haystack = [pkg.name, pkg.display_name, pkg.description, pkg.publisher, pkg.image_ref, ...(pkg.tags || [])].join(' ').toLowerCase()
  const result: string[] = []
  if (/(mock|stub|virtual service|simulat)/.test(haystack)) result.push('mock')
  if (/(contract|openapi|proto|graphql|schema|asyncapi)/.test(haystack)) result.push('contract')
  if (/(grpc|proto)/.test(haystack)) result.push('grpc')
  if (/(async|event|kafka|nats|queue|stream)/.test(haystack)) result.push('async')
  return Array.from(new Set(result))
}

function registryBadge(kind: string) {
  if (kind === 'ghcr') return <span className={`${styles.badge} ${styles.badgeRegistryPrimary}`}>Hosted</span>
  if (kind === 'jfrog') return <span className={`${styles.badge} ${styles.badgeRegistrySecondary}`}>Private</span>
  return <span className={`${styles.badge} ${styles.badgeNeutral}`}>{kind || 'registry'}</span>
}

export default function Catalog() {
  const nav = useNavigate()
  const admin = isAdmin()
  const [packages, setPackages] = useState<CatalogPackage[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [totalPages, setTotalPages] = useState(1)
  const [search, setSearch] = useState('')
  const [registry, setRegistry] = useState('')
  const [capability, setCapability] = useState('')
  const [publisher, setPublisher] = useState('')
  const [loading, setLoading] = useState(false)
  const [launching, setLaunching] = useState('')
  const [launchPkg, setLaunchPkg] = useState<CatalogPackage | null>(null)
  const [profileValue, setProfileValue] = useState('')
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const load = async (nextPage: number, q: string, reg: string) => {
    setLoading(true)
    try {
      const params = new URLSearchParams({ page: String(nextPage), page_size: '36' })
      if (q) params.set('q', q)
      if (reg) params.set('registry', reg)
      const res = await fetch(`${API}/api/catalog?${params}`, { headers: authHeaders() })
      if (!res.ok) return
      const data: CatalogResponse = await res.json()
      setPackages(data.packages || [])
      setTotal(data.total || 0)
      setTotalPages(data.total_pages || 1)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void load(page, search, registry) }, [page, registry])

  const visiblePackages = useMemo(() => {
    return packages.filter(pkg => {
      const pkgCaps = capsForPackage(pkg)
      if (capability && !pkgCaps.includes(capability)) return false
      if (publisher && (pkg.publisher || 'Unknown') !== publisher) return false
      return true
    })
  }, [packages, capability, publisher])

  const publishers = useMemo(() => {
    const counts = new Map<string, number>()
    packages.forEach(pkg => counts.set(pkg.publisher || 'Unknown', (counts.get(pkg.publisher || 'Unknown') || 0) + 1))
    return Array.from(counts.entries()).sort((a, b) => b[1] - a[1]).slice(0, 6)
  }, [packages])

  const stats = useMemo(() => ({
    visible: visiblePackages.length,
    mockReady: visiblePackages.filter(pkg => capsForPackage(pkg).includes('mock')).length,
    contractReady: visiblePackages.filter(pkg => capsForPackage(pkg).includes('contract')).length,
    publishers: new Set(visiblePackages.map(pkg => pkg.publisher || 'Unknown')).size,
  }), [visiblePackages])

  const handleSearch = (value: string) => {
    setSearch(value)
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => {
      setPage(1)
      void load(1, value, registry)
    }, 300)
  }

  const openLaunch = (pkg: CatalogPackage) => {
    setLaunchPkg(pkg)
    setProfileValue(pkg.default_profile || pkg.profiles?.[0] || '')
  }

  const startRun = async (pkg: CatalogPackage, profile: string) => {
    setLaunching(pkg.package_id)
    try {
      const res = await fetch(`${API}/api/runs`, {
        method: 'POST',
        headers: authHeaders(),
        body: JSON.stringify({ package_id: pkg.package_id, profile: profile.trim() || undefined }),
      })
      const data = await res.json().catch(() => ({}))
      if (res.ok && data.run_id) {
        setLaunchPkg(null)
        nav(`/runs/${data.run_id}`)
      }
    } finally {
      setLaunching('')
    }
  }

  return (
    <Layout>
      <Page title='Suites' toolbar={admin ? <button className='app-button app-button--secondary' onClick={() => nav('/settings/catalog?tab=packages')}><FaGear />Manage Catalog</button> : undefined}>
        <section className={styles.hero}>
          <div>
            <div className={styles.eyebrow}>Enabled suites</div>
            <h1>Browse the suites your admins made available.</h1>
            <p>Choose an enabled suite, pick its profile, and launch the run without exposing the raw catalog to every user.</p>
          </div>
          <div className={styles.heroStats}>
            <StatCard label='Visible suites' value={String(stats.visible)} />
            <StatCard label='Mock-ready' value={String(stats.mockReady)} />
            <StatCard label='Contract-first' value={String(stats.contractReady)} />
            <StatCard label='Publishers' value={String(stats.publishers)} />
          </div>
        </section>

        <div className={styles.searchRow}>
          <input className={styles.searchInput} type='text' value={search} onChange={e => handleSearch(e.target.value)} placeholder='Search enabled suites by name, publisher, tags, or profile...' />
          <span className={styles.count}>{loading ? 'Loading...' : `${visiblePackages.length} visible / ${total} total`}</span>
        </div>

        <div className={styles.layout}>
          <aside className={styles.filters}>
            <div className={styles.filterGroup}>
              <div className={styles.filterLabel}>Registry</div>
              {[
                { value: '', label: 'All' },
                { value: 'ghcr', label: 'Hosted' },
                { value: 'jfrog', label: 'Private' },
              ].map(option => (
                <label key={option.value} className={styles.filterOption}>
                  <input type='radio' name='registry' checked={registry === option.value} onChange={() => { setRegistry(option.value); setPage(1) }} />
                  {option.label}
                </label>
              ))}
            </div>

            <div className={styles.filterGroup}>
              <div className={styles.filterLabel}>Capability</div>
              {capabilities.map(option => (
                <label key={option.value || 'all'} className={styles.filterOption}>
                  <input type='radio' name='capability' checked={capability === option.value} onChange={() => setCapability(option.value)} />
                  {option.label}
                </label>
              ))}
            </div>

            {publishers.length > 0 && (
              <div className={styles.filterGroup}>
                <div className={styles.filterLabel}>Publisher</div>
                <button className={`${styles.publisherChip}${publisher === '' ? ` ${styles.publisherChipActive}` : ''}`} onClick={() => setPublisher('')}>All</button>
                {publishers.map(([name, count]) => (
                  <button key={name} className={`${styles.publisherChip}${publisher === name ? ` ${styles.publisherChipActive}` : ''}`} onClick={() => setPublisher(name)}>
                    {name} <span>{count}</span>
                  </button>
                ))}
              </div>
            )}
          </aside>

          <div className={styles.list}>
            {loading ? (
              <div className='app-empty-state'>Loading packages...</div>
            ) : visiblePackages.length === 0 ? (
              <div className='panel-card'>
                <div className='empty-state'>
                  <div className='empty-state__icon'><FaBoxOpen /></div>
                  <h4>No suites found</h4>
                  <p>Try broadening the search, clearing a capability filter, or ask an admin to enable more suites from the catalog.</p>
                </div>
              </div>
            ) : (
              <>
                <div className={styles.grid}>
                  {visiblePackages.map(pkg => (
                    <article key={pkg.package_id} className={styles.card}>
                      <div className={styles.cardTop}>
                        <div className={styles.cardIcon}><FaBoxOpen /></div>
                        <div className={styles.cardMeta}>
                          <div className={styles.cardName}>{pkg.display_name || pkg.name}</div>
                          <div className={styles.cardPublisher}>{pkg.publisher || 'Unknown publisher'}</div>
                        </div>
                        {registryBadge(pkg.registry_kind)}
                      </div>

                      <p className={styles.cardDesc}>{pkg.description || 'Containerized suite package ready for orchestration.'}</p>

                      <div className={styles.signalRow}>
                        {capsForPackage(pkg).map(signal => <span key={signal} className={`${styles.badge} ${styles.badgeSignal}`}>{signal}</span>)}
                        {(pkg.tags || []).slice(0, 3).map(tag => <span key={tag} className={`${styles.badge} ${styles.badgeNeutral}`}>{tag}</span>)}
                        {pkg.version && <span className={`${styles.badge} ${styles.badgeVersion}`}>{pkg.version}</span>}
                      </div>

                      {(pkg.service_count || pkg.mock_count || pkg.test_count || pkg.contract_count) ? (
                        <div className={styles.componentRow}>
                          {pkg.service_count ? <span className={styles.componentChip}>{pkg.service_count} services</span> : null}
                          {pkg.mock_count ? <span className={styles.componentChip}>{pkg.mock_count} mock APIs</span> : null}
                          {pkg.test_count ? <span className={styles.componentChip}>{pkg.test_count} test runners</span> : null}
                          {pkg.contract_count ? <span className={styles.componentChip}>{pkg.contract_count} contracts</span> : null}
                          {pkg.profiles?.length ? <span className={styles.componentChip}>{pkg.profiles.length} profiles</span> : null}
                        </div>
                      ) : null}

                      <div className={styles.cardInfo}>
                        <span>{pkg.image_ref}</span>
                        <span>Updated {new Date(pkg.updated_at).toLocaleDateString()}</span>
                      </div>

                      <div className={styles.cardActions}>
                        <button className='app-button app-button--primary' onClick={() => openLaunch(pkg)} disabled={launching === pkg.package_id}>
                          <FaPlay />
                          {launching === pkg.package_id ? 'Launching...' : 'Launch suite'}
                        </button>
                      </div>
                    </article>
                  ))}
                </div>

                {totalPages > 1 && (
                  <div className={styles.pagination}>
                    <button className='app-button app-button--secondary' disabled={page <= 1} onClick={() => setPage(p => p - 1)}>Prev</button>
                    <span>Page {page} of {totalPages}</span>
                    <button className='app-button app-button--secondary' disabled={page >= totalPages} onClick={() => setPage(p => p + 1)}>Next</button>
                  </div>
                )}
              </>
            )}
          </div>
        </div>
        {launchPkg && (
          <div className={styles.overlay} onClick={() => launching ? null : setLaunchPkg(null)}>
            <div className={styles.modal} onClick={event => event.stopPropagation()}>
              <div className={styles.modalHeader}>
                <div>
                  <div className={styles.modalEyebrow}>Launch suite</div>
                  <h3>{launchPkg.display_name || launchPkg.name}</h3>
                </div>
              </div>
              <p className={styles.modalCopy}>Choose the suite profile for this run. If the package publishes known profiles, you can pick one directly. Otherwise you can type the profile name yourself.</p>
              {launchPkg.profiles && launchPkg.profiles.length > 0 ? (
                <label className={styles.modalField}>
                  <span>Suite profile</span>
                  <select className={styles.modalInput} value={profileValue} onChange={event => setProfileValue(event.target.value)}>
                    {launchPkg.profiles.map(profile => <option key={profile} value={profile}>{profile}</option>)}
                  </select>
                </label>
              ) : (
                <label className={styles.modalField}>
                  <span>Suite profile</span>
                  <input className={styles.modalInput} value={profileValue} onChange={event => setProfileValue(event.target.value)} placeholder='default' />
                </label>
              )}
              <div className={styles.modalMeta}>
                <span>{launchPkg.image_ref}</span>
                {launchPkg.default_profile ? <span>Default: {launchPkg.default_profile}</span> : null}
              </div>
              <div className={styles.modalActions}>
                <button className='app-button app-button--secondary' onClick={() => setLaunchPkg(null)} disabled={launching === launchPkg.package_id}>Cancel</button>
                <button className='app-button app-button--primary' onClick={() => void startRun(launchPkg, profileValue)} disabled={launching === launchPkg.package_id}>
                  <FaPlay />
                  {launching === launchPkg.package_id ? 'Launching...' : 'Start run'}
                </button>
              </div>
            </div>
          </div>
        )}
      </Page>
    </Layout>
  )
}

function StatCard({ label, value }: { label: string; value: string }) {
  return <div className={styles.statCard}><span>{label}</span><strong>{value}</strong></div>
}
