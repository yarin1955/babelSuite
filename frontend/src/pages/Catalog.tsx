import { useEffect, useRef, useState } from 'react'
import { FaBoxOpen, FaGithub, FaFrog, FaGear } from 'react-icons/fa6'
import { useNavigate } from 'react-router-dom'
import Layout from '../components/Layout'
import Page from '../components/Page'
import styles from './Catalog.module.css'

const API = 'http://localhost:8090'

function authHeaders() {
    return { 'Content-Type': 'application/json', Authorization: `Bearer ${localStorage.getItem('token')}` }
}

interface CatalogPackage {
    package_id:    string
    org_id:        string
    registry_id:   string
    registry_kind: string
    name:          string
    display_name:  string
    description:   string
    publisher:     string
    image_ref:     string
    version:       string
    tags:          string[]
    enabled:       boolean
    updated_at:    string
}

interface CatalogResponse {
    packages:    CatalogPackage[]
    total:       number
    page:        number
    page_size:   number
    total_pages: number
}

function RegistryBadge({ kind }: { kind: string }) {
    if (kind === 'ghcr') return (
        <span className={`${styles.badge} ${styles.badgeGhcr}`}>
            <FaGithub /> GHCR
        </span>
    )
    if (kind === 'jfrog') return (
        <span className={`${styles.badge} ${styles.badgeJfrog}`}>
            <FaFrog /> JFrog
        </span>
    )
    return <span className={`${styles.badge} ${styles.badgeTag}`}>{kind}</span>
}

function PackageCard({ pkg }: { pkg: CatalogPackage }) {
    const displayName = pkg.display_name || pkg.name
    return (
        <div className={styles.card}>
            <div className={styles.cardHeader}>
                <div className={styles.cardIcon}>
                    <FaBoxOpen />
                </div>
                <div className={styles.cardMeta}>
                    <div className={styles.cardName} title={displayName}>{displayName}</div>
                    {pkg.publisher && (
                        <div className={styles.cardPublisher}>{pkg.publisher}</div>
                    )}
                </div>
            </div>
            {pkg.description && (
                <div className={styles.cardDesc}>{pkg.description}</div>
            )}
            <div className={styles.cardFooter}>
                <RegistryBadge kind={pkg.registry_kind} />
                {(pkg.tags || []).slice(0, 3).map(t => (
                    <span key={t} className={`${styles.badge} ${styles.badgeTag}`}>{t}</span>
                ))}
                {pkg.version && (
                    <span className={`${styles.badge} ${styles.badgeVersion}`}>{pkg.version}</span>
                )}
            </div>
        </div>
    )
}

function isAdmin(): boolean {
    try { return JSON.parse(localStorage.getItem('user') || '{}').is_admin === true } catch { return false }
}

export default function Catalog() {
    const nav = useNavigate()
    const admin = isAdmin()
    const [packages, setPackages] = useState<CatalogPackage[]>([])
    const [total, setTotal]       = useState(0)
    const [page, setPage]         = useState(1)
    const [totalPages, setTotalPages] = useState(1)
    const [search, setSearch]     = useState('')
    const [registry, setRegistry] = useState('')
    const [loading, setLoading]   = useState(false)
    const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

    const load = async (p: number, q: string, reg: string) => {
        setLoading(true)
        try {
            const params = new URLSearchParams({ page: String(p), page_size: '20' })
            if (q)   params.set('q', q)
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

    useEffect(() => {
        load(page, search, registry)
    }, [page, registry])

    const handleSearch = (val: string) => {
        setSearch(val)
        if (debounceRef.current) clearTimeout(debounceRef.current)
        debounceRef.current = setTimeout(() => {
            setPage(1)
            load(1, val, registry)
        }, 350)
    }

    const handleRegistry = (val: string) => {
        setRegistry(val)
        setPage(1)
    }

    return (
        <Layout>
            <Page title='Catalog' toolbar={admin ? (
                <button
                    onClick={() => nav('/admin/catalog')}
                    style={{ display: 'inline-flex', alignItems: 'center', gap: 6, padding: '5px 12px', borderRadius: 5, border: '1px solid #e5e7eb', background: '#fff', fontSize: 13, cursor: 'pointer', color: '#374151' }}
                >
                    <FaGear /> Manage
                </button>
            ) : undefined}>
                <div className={styles.searchWrap}>
                    <input
                        className={styles.searchInput}
                        type='text'
                        placeholder='Search packages…'
                        value={search}
                        onChange={e => handleSearch(e.target.value)}
                    />
                    <span className={styles.count}>
                        {loading ? 'Loading…' : `${total} package${total !== 1 ? 's' : ''}`}
                    </span>
                </div>

                <div className={styles.layout}>
                    <aside className={styles.filters}>
                        <p className={styles.filtersTitle}>Filters</p>

                        <div className={styles.filterGroup}>
                            <div className={styles.filterGroupLabel}>Registry</div>
                            {[
                                { value: '',      label: 'All' },
                                { value: 'ghcr',  label: 'GitHub (GHCR)' },
                                { value: 'jfrog', label: 'JFrog' },
                            ].map(opt => (
                                <label key={opt.value} className={styles.filterOption}>
                                    <input
                                        type='radio'
                                        name='registry'
                                        value={opt.value}
                                        checked={registry === opt.value}
                                        onChange={() => handleRegistry(opt.value)}
                                    />
                                    {opt.label}
                                </label>
                            ))}
                        </div>
                    </aside>

                    <div className={styles.list}>
                        {loading ? (
                            <div className={styles.loading}>
                                <div className={styles.spinner} /> Loading packages…
                            </div>
                        ) : packages.length === 0 ? (
                            <div className='empty-state'>
                                <div className='empty-state__icon'><FaBoxOpen /></div>
                                <h4>No packages found</h4>
                                <p>
                                    {search
                                        ? 'No packages match your search.'
                                        : 'Ask an admin to connect a registry and enable packages.'}
                                </p>
                            </div>
                        ) : (
                            <>
                                <div className={styles.grid}>
                                    {packages.map(pkg => (
                                        <PackageCard key={pkg.package_id} pkg={pkg} />
                                    ))}
                                </div>

                                {totalPages > 1 && (
                                    <div className={styles.pagination}>
                                        <button
                                            className={styles.pageBtn}
                                            disabled={page <= 1}
                                            onClick={() => setPage(p => p - 1)}
                                        >‹</button>

                                        {Array.from({ length: totalPages }, (_, i) => i + 1)
                                            .filter(n => n === 1 || n === totalPages || Math.abs(n - page) <= 2)
                                            .reduce<(number | '…')[]>((acc, n, i, arr) => {
                                                if (i > 0 && n - (arr[i - 1] as number) > 1) acc.push('…')
                                                acc.push(n)
                                                return acc
                                            }, [])
                                            .map((n, i) => n === '…'
                                                ? <span key={`e${i}`} className={styles.pageBtn} style={{ cursor: 'default' }}>…</span>
                                                : (
                                                    <button
                                                        key={n}
                                                        className={`${styles.pageBtn}${page === n ? ' ' + styles.pageBtnActive : ''}`}
                                                        onClick={() => setPage(n as number)}
                                                    >{n}</button>
                                                )
                                            )
                                        }

                                        <button
                                            className={styles.pageBtn}
                                            disabled={page >= totalPages}
                                            onClick={() => setPage(p => p + 1)}
                                        >›</button>
                                    </div>
                                )}
                            </>
                        )}
                    </div>
                </div>
            </Page>
        </Layout>
    )
}
