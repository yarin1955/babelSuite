import { useEffect, useRef, useState } from 'react'
import { FaPlus, FaPen, FaTrash, FaRotate } from 'react-icons/fa6'
import { useNavigate, useSearchParams } from 'react-router-dom'
import Layout from '../components/Layout'
import Page from '../components/Page'
import styles from './AdminCatalog.module.css'

const API = 'http://localhost:8090'

function authHeaders(extra?: Record<string, string>) {
    return {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${localStorage.getItem('token')}`,
        ...extra,
    }
}

/* ── Types ───────────────────────────────────────────────────────────────── */
interface Registry {
    registry_id: string
    org_id:      string
    kind:        string
    name:        string
    url:         string
    insecure_skip_tls_verify?: boolean
    username?: string
    has_password?: boolean
    has_bearer_token?: boolean
    has_tls_ca_data?: boolean
    has_tls_cert_data?: boolean
    has_tls_key_data?: boolean
    enabled:     boolean
    created_at:  string
}

interface CatalogPackage {
    package_id:    string
    name:          string
    display_name:  string
    description:   string
    publisher:     string
    image_ref:     string
    version:       string
    registry_kind: string
    tags:          string[]
    profiles?:     string[]
    default_profile?: string
    enabled:       boolean
    updated_at:    string
}

/* ── Registry kind badge ─────────────────────────────────────────────────── */
function KindBadge({ kind }: { kind: string }) {
    if (kind === 'ghcr')  return <span className={`${styles.badge} ${styles.badgeRegistryPrimary}`}>Hosted</span>
    if (kind === 'jfrog') return <span className={`${styles.badge} ${styles.badgeRegistrySecondary}`}>Private</span>
    return <span className={styles.badge}>{kind}</span>
}

/* ── Toggle switch ───────────────────────────────────────────────────────── */
function Toggle({ checked, onChange }: { checked: boolean; onChange: () => void }) {
    return (
        <label className={styles.toggle}>
            <input type='checkbox' checked={checked} onChange={onChange} />
            <span className={styles.toggleSlider} />
        </label>
    )
}

/* ── Registry modal ──────────────────────────────────────────────────────── */
interface RegistryForm {
    name: string
    kind: string
    url: string
    username: string
    password: string
    bearer_token: string
    tls_ca_data: string
    tls_cert_data: string
    tls_key_data: string
    clear_password: boolean
    clear_bearer_token: boolean
    clear_tls_ca_data: boolean
    clear_tls_cert_data: boolean
    clear_tls_key_data: boolean
    insecure_skip_tls_verify: boolean
    enabled: boolean
}

function connectionSummary(registry: Registry) {
    const items: string[] = []
    if (registry.username && registry.has_password) items.push('Basic auth')
    if (registry.has_bearer_token) items.push('Token')
    if (registry.has_tls_ca_data) items.push('Custom CA')
    if (registry.has_tls_cert_data && registry.has_tls_key_data) items.push('Client cert')
    if (registry.insecure_skip_tls_verify) items.push('TLS verify off')
    return items.length ? items.join(' · ') : 'None'
}

function connectionDetailsSummary(registry: Registry) {
    void connectionSummary(registry)
    const items: string[] = []
    if (registry.username && registry.has_password) items.push('Basic auth')
    if (registry.has_bearer_token) items.push('Token')
    if (registry.has_tls_ca_data) items.push('Custom CA')
    if (registry.has_tls_cert_data && registry.has_tls_key_data) items.push('Client cert')
    if (registry.insecure_skip_tls_verify) items.push('TLS verify off')
    return items.length ? items.join(' | ') : 'None'
}

function RegistryModal({
    initial, onSave, onClose,
}: {
    initial?: Registry
    onSave: (form: RegistryForm) => Promise<void>
    onClose: () => void
}) {
    const [form, setForm] = useState<RegistryForm>({
        name: initial?.name ?? '',
        kind: initial?.kind ?? 'ghcr',
        url:  initial?.url  ?? '',
        username: initial?.username ?? '',
        password: '',
        bearer_token: '',
        tls_ca_data: '',
        tls_cert_data: '',
        tls_key_data: '',
        clear_password: false,
        clear_bearer_token: false,
        clear_tls_ca_data: false,
        clear_tls_cert_data: false,
        clear_tls_key_data: false,
        insecure_skip_tls_verify: initial?.insecure_skip_tls_verify ?? false,
        enabled: initial?.enabled ?? true,
    })
    const [saving, setSaving] = useState(false)

    const set = (k: keyof RegistryForm) => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) =>
        setForm(f => ({ ...f, [k]: e.target.value }))

    const submit = async (e: React.FormEvent) => {
        e.preventDefault()
        setSaving(true)
        await onSave(form)
        setSaving(false)
    }

    return (
        <div className={styles.overlay} onClick={onClose}>
            <div className={styles.modal} onClick={e => e.stopPropagation()}>
                <h3 className={styles.modalTitle}>{initial ? 'Edit Registry' : 'Add Registry'}</h3>
                <form onSubmit={submit}>
                    <div className={styles.field}>
                        <label className={styles.fieldLabel}>Name</label>
                        <input className={styles.fieldInput} value={form.name} onChange={set('name')} required placeholder='My Registry' />
                    </div>
                    <div className={styles.field}>
                        <label className={styles.fieldLabel}>Type</label>
                        <select className={styles.fieldSelect} value={form.kind} onChange={set('kind')}>
                            <option value='ghcr'>Hosted registry</option>
                            <option value='jfrog'>Private registry</option>
                        </select>
                    </div>
                    <div className={styles.field}>
                        <label className={styles.fieldLabel}>URL (optional)</label>
                        <input className={styles.fieldInput} value={form.url} onChange={set('url')} placeholder='https://registry.example.com' />
                    </div>
                    <div className={styles.field}>
                        <label className={styles.fieldLabel}>Username</label>
                        <input className={styles.fieldInput} value={form.username} onChange={set('username')} placeholder='optional' />
                    </div>
                    <div className={styles.field}>
                        <label className={styles.fieldLabel}>Password {initial?.has_password && '(leave blank to keep existing)'}</label>
                        <input className={styles.fieldInput} type='password' value={form.password} onChange={set('password')} placeholder={initial?.has_password ? 'Stored separately' : 'optional'} />
                        {initial?.has_password && (
                            <label className={styles.checkboxField}>
                                <input type='checkbox' checked={form.clear_password} onChange={e => setForm(f => ({ ...f, clear_password: e.target.checked, password: e.target.checked ? '' : f.password }))} />
                                <span>Clear stored password</span>
                            </label>
                        )}
                    </div>
                    <div className={styles.field}>
                        <label className={styles.fieldLabel}>Token {initial?.has_bearer_token && '(leave blank to keep existing)'}</label>
                        <textarea className={`${styles.fieldInput} ${styles.fieldTextarea}`} value={form.bearer_token} onChange={e => setForm(f => ({ ...f, bearer_token: e.target.value, clear_bearer_token: false }))} placeholder={initial?.has_bearer_token ? 'Stored separately' : 'optional'} rows={3} />
                        {initial?.has_bearer_token && (
                            <label className={styles.checkboxField}>
                                <input type='checkbox' checked={form.clear_bearer_token} onChange={e => setForm(f => ({ ...f, clear_bearer_token: e.target.checked, bearer_token: e.target.checked ? '' : f.bearer_token }))} />
                                <span>Clear stored token</span>
                            </label>
                        )}
                    </div>
                    <div className={styles.securityGrid}>
                        <div className={styles.field}>
                            <label className={styles.fieldLabel}>CA certificate</label>
                            <textarea className={`${styles.fieldInput} ${styles.fieldTextarea}`} value={form.tls_ca_data} onChange={e => setForm(f => ({ ...f, tls_ca_data: e.target.value, clear_tls_ca_data: false }))} placeholder={initial?.has_tls_ca_data ? 'Stored PEM data' : '-----BEGIN CERTIFICATE-----'} rows={4} />
                            {initial?.has_tls_ca_data && (
                                <label className={styles.checkboxField}>
                                    <input type='checkbox' checked={form.clear_tls_ca_data} onChange={e => setForm(f => ({ ...f, clear_tls_ca_data: e.target.checked, tls_ca_data: e.target.checked ? '' : f.tls_ca_data }))} />
                                    <span>Clear stored CA bundle</span>
                                </label>
                            )}
                        </div>
                        <div className={styles.field}>
                            <label className={styles.fieldLabel}>Client certificate</label>
                            <textarea className={`${styles.fieldInput} ${styles.fieldTextarea}`} value={form.tls_cert_data} onChange={e => setForm(f => ({ ...f, tls_cert_data: e.target.value, clear_tls_cert_data: false }))} placeholder={initial?.has_tls_cert_data ? 'Stored PEM data' : '-----BEGIN CERTIFICATE-----'} rows={4} />
                        </div>
                        <div className={styles.field}>
                            <label className={styles.fieldLabel}>Client key</label>
                            <textarea className={`${styles.fieldInput} ${styles.fieldTextarea}`} value={form.tls_key_data} onChange={e => setForm(f => ({ ...f, tls_key_data: e.target.value, clear_tls_key_data: false }))} placeholder={initial?.has_tls_key_data ? 'Stored PEM key' : '-----BEGIN PRIVATE KEY-----'} rows={4} />
                            {(initial?.has_tls_cert_data || initial?.has_tls_key_data) && (
                                <label className={styles.checkboxField}>
                                    <input type='checkbox' checked={form.clear_tls_cert_data || form.clear_tls_key_data} onChange={e => setForm(f => ({ ...f, clear_tls_cert_data: e.target.checked, clear_tls_key_data: e.target.checked, tls_cert_data: e.target.checked ? '' : f.tls_cert_data, tls_key_data: e.target.checked ? '' : f.tls_key_data }))} />
                                    <span>Clear stored client certificate pair</span>
                                </label>
                            )}
                        </div>
                    </div>
                    <label className={styles.checkboxField}>
                        <input type='checkbox' checked={form.insecure_skip_tls_verify} onChange={e => setForm(f => ({ ...f, insecure_skip_tls_verify: e.target.checked }))} />
                        <span>Skip TLS verification</span>
                    </label>
                    <div className={styles.modalFooter}>
                        <button type='button' className={`${styles.btn} ${styles.btnSecondary}`} onClick={onClose}>Cancel</button>
                        <button type='submit' className={`${styles.btn} ${styles.btnPrimary}`} disabled={saving}>
                            {saving ? 'Saving…' : 'Save'}
                        </button>
                    </div>
                </form>
            </div>
        </div>
    )
}

/* ── Sync modal ──────────────────────────────────────────────────────────── */
function SyncModal({ registry, onClose }: { registry: Registry; onClose: () => void }) {
    const [repos, setRepos] = useState('')
    const [syncing, setSyncing] = useState(false)
    const [result, setResult]   = useState<string | null>(null)

    const submit = async (e: React.FormEvent) => {
        e.preventDefault()
        const list = repos.split('\n').map(s => s.trim()).filter(Boolean)
        if (!list.length) return
        setSyncing(true)
        try {
            const res = await fetch(`${API}/api/admin/registries/${registry.registry_id}/sync`, {
                method:  'POST',
                headers: authHeaders(),
                body:    JSON.stringify({ repos: list }),
            })
            const data = await res.json()
            setResult(res.ok ? `Synced ${data.synced} package(s).` : data.error || 'Sync failed')
        } finally {
            setSyncing(false)
        }
    }

    return (
        <div className={styles.overlay} onClick={onClose}>
            <div className={styles.modal} onClick={e => e.stopPropagation()}>
                <h3 className={styles.modalTitle}>Sync — {registry.name}</h3>
                {result ? (
                    <>
                        <p className={styles.syncCount}>{result}</p>
                        <div className={styles.modalFooter}>
                            <button className={`${styles.btn} ${styles.btnPrimary}`} onClick={onClose}>Close</button>
                        </div>
                    </>
                ) : (
                    <form onSubmit={submit}>
                        <div className={styles.field}>
                            <label className={styles.fieldLabel}>Repository paths (one per line)</label>
                            <textarea
                                style={{ width: '100%', height: 100, padding: '8px 10px', borderRadius: 5, border: '1px solid #e5e7eb', fontSize: 13, resize: 'vertical', boxSizing: 'border-box', outline: 'none' }}
                                value={repos}
                                onChange={e => setRepos(e.target.value)}
                                placeholder={'my-org/my-suite\nmy-org/another-suite'}
                            />
                        </div>
                        <div className={styles.modalFooter}>
                            <button type='button' className={`${styles.btn} ${styles.btnSecondary}`} onClick={onClose}>Cancel</button>
                            <button type='submit' className={`${styles.btn} ${styles.btnPrimary}`} disabled={syncing}>
                                {syncing ? 'Syncing…' : 'Sync'}
                            </button>
                        </div>
                    </form>
                )}
            </div>
        </div>
    )
}

/* ── Registries tab ──────────────────────────────────────────────────────── */
function RegistriesTab() {
    const [regs, setRegs] = useState<Registry[]>([])
    const [modal, setModal] = useState<'add' | Registry | null>(null)
    const [syncTarget, setSyncTarget] = useState<Registry | null>(null)

    const load = async () => {
        const res = await fetch(`${API}/api/admin/registries`, { headers: authHeaders() })
        if (res.ok) setRegs(await res.json() ?? [])
    }

    useEffect(() => { load() }, [])

    const save = async (form: RegistryForm) => {
        const isEdit = modal !== 'add' && modal !== null
        const url    = isEdit ? `${API}/api/admin/registries/${(modal as Registry).registry_id}` : `${API}/api/admin/registries`
        const res    = await fetch(url, { method: isEdit ? 'PUT' : 'POST', headers: authHeaders(), body: JSON.stringify(form) })
        if (res.ok) { await load(); setModal(null) }
    }

    const del = async (id: string) => {
        if (!confirm('Delete this registry?')) return
        await fetch(`${API}/api/admin/registries/${id}`, { method: 'DELETE', headers: authHeaders() })
        await load()
    }

    return (
        <>
            <div className={styles.notePanel}>
                <strong>Catalog sources only</strong>
                <p>These registries import suites into the catalog. Execution targets are configured on agent tokens from the Agents page.</p>
            </div>
            <div className={styles.toolbar}>
                <span className={styles.toolbarTitle}>Registries</span>
                <button className={`${styles.btn} ${styles.btnPrimary}`} onClick={() => setModal('add')}>
                    <FaPlus /> Add Registry
                </button>
            </div>

            {regs.length === 0 ? (
                <p style={{ color: '#6d7f8b', fontSize: 14 }}>No registries configured yet.</p>
            ) : (
                <div className='white-box'>
                    <table className={styles.table}>
                        <thead>
                            <tr>
                                <th>Name</th>
                                <th>Type</th>
                                <th>URL</th>
                                <th>Connection</th>
                                <th>Actions</th>
                            </tr>
                        </thead>
                        <tbody>
                            {regs.map(r => (
                                <tr key={r.registry_id}>
                                    <td>{r.name}</td>
                                    <td><KindBadge kind={r.kind} /></td>
                                    <td style={{ color: '#6d7f8b', fontSize: 12 }}>{r.url || '—'}</td>
                                    <td style={{ color: '#6d7f8b', fontSize: 12 }}>{connectionDetailsSummary(r)}</td>
                                    <td>
                                        <div className={styles.actions}>
                                            <button
                                                className={`${styles.btn} ${styles.btnSecondary} ${styles.btnIcon}`}
                                                title='Sync packages'
                                                onClick={() => setSyncTarget(r)}
                                            ><FaRotate /></button>
                                            <button
                                                className={`${styles.btn} ${styles.btnSecondary} ${styles.btnIcon}`}
                                                title='Edit'
                                                onClick={() => setModal(r)}
                                            ><FaPen /></button>
                                            <button
                                                className={`${styles.btn} ${styles.btnDanger} ${styles.btnIcon}`}
                                                title='Delete'
                                                onClick={() => del(r.registry_id)}
                                            ><FaTrash /></button>
                                        </div>
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            )}

            {modal && (
                <RegistryModal
                    initial={modal === 'add' ? undefined : modal as Registry}
                    onSave={save}
                    onClose={() => setModal(null)}
                />
            )}
            {syncTarget && (
                <SyncModal registry={syncTarget} onClose={() => setSyncTarget(null)} />
            )}
        </>
    )
}

/* ── Packages tab ────────────────────────────────────────────────────────── */
function PackagesTab() {
    const [packages, setPackages] = useState<CatalogPackage[]>([])
    const [total, setTotal]       = useState(0)
    const [page, setPage]         = useState(1)
    const [search, setSearch]     = useState('')
    const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

    const load = async (p: number, q: string) => {
        const params = new URLSearchParams({ page: String(p), page_size: '50' })
        if (q) params.set('q', q)
        const res = await fetch(`${API}/api/admin/catalog?${params}`, { headers: authHeaders() })
        if (!res.ok) return
        const data = await res.json()
        setPackages(data.packages || [])
        setTotal(data.total || 0)
    }

    useEffect(() => { load(page, search) }, [page])

    const handleSearch = (val: string) => {
        setSearch(val)
        if (debounceRef.current) clearTimeout(debounceRef.current)
        debounceRef.current = setTimeout(() => { setPage(1); load(1, val) }, 350)
    }

    const toggle = async (pkg: CatalogPackage) => {
        const ep = pkg.enabled
            ? `${API}/api/admin/catalog/${pkg.package_id}/disable`
            : `${API}/api/admin/catalog/${pkg.package_id}/enable`
        const res = await fetch(ep, { method: 'POST', headers: authHeaders() })
        if (res.ok) setPackages(prev => prev.map(p => p.package_id === pkg.package_id ? { ...p, enabled: !p.enabled } : p))
    }

    const del = async (pkg: CatalogPackage) => {
        if (!confirm(`Delete "${pkg.display_name || pkg.name}"?`)) return
        await fetch(`${API}/api/admin/catalog/${pkg.package_id}`, { method: 'DELETE', headers: authHeaders() })
        await load(page, search)
    }

    return (
        <>
            <div className={styles.notePanel}>
                <strong>Enable suites for users</strong>
                <p>Only enabled packages appear on the Suites page. Published suite profiles are shown here so you can see what launch options users will get before turning a package on.</p>
            </div>
            <div className={styles.toolbar}>
                <span className={styles.toolbarTitle}>{total} packages</span>
                <input
                    className={styles.searchInput}
                    type='text'
                    placeholder='Search…'
                    value={search}
                    onChange={e => handleSearch(e.target.value)}
                />
            </div>

            {packages.length === 0 ? (
                <p style={{ color: '#6d7f8b', fontSize: 14 }}>
                    No packages yet. Sync a registry to import packages.
                </p>
            ) : (
                <div className='white-box'>
                    <table className={styles.table}>
                        <thead>
                            <tr>
                                <th>Name</th>
                                <th>Registry</th>
                                <th>Version</th>
                                <th>Publisher</th>
                                <th>Profiles</th>
                                <th>Enabled</th>
                                <th></th>
                            </tr>
                        </thead>
                        <tbody>
                            {packages.map(pkg => (
                                <tr key={pkg.package_id}>
                                    <td>
                                        <div style={{ fontWeight: 500 }}>{pkg.display_name || pkg.name}</div>
                                        {pkg.description && (
                                            <div style={{ fontSize: 12, color: '#6d7f8b', marginTop: 2, maxWidth: 300, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                                                {pkg.description}
                                            </div>
                                        )}
                                    </td>
                                    <td><KindBadge kind={pkg.registry_kind} /></td>
                                    <td style={{ fontSize: 12, color: '#6d7f8b' }}>{pkg.version || '—'}</td>
                                    <td style={{ fontSize: 12, color: '#6d7f8b' }}>{pkg.publisher || '—'}</td>
                                    <td style={{ fontSize: 12, color: '#6d7f8b' }}>
                                        {pkg.profiles?.length
                                            ? `${pkg.profiles.length} profile${pkg.profiles.length === 1 ? '' : 's'}${pkg.default_profile ? ` / default ${pkg.default_profile}` : ''}`
                                            : 'none published'}
                                    </td>
                                    <td><Toggle checked={pkg.enabled} onChange={() => toggle(pkg)} /></td>
                                    <td>
                                        <button
                                            className={`${styles.btn} ${styles.btnDanger} ${styles.btnIcon}`}
                                            title='Delete'
                                            onClick={() => del(pkg)}
                                        ><FaTrash /></button>
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            )}
        </>
    )
}

/* ── Page ────────────────────────────────────────────────────────────────── */
export default function AdminCatalog() {
    const nav = useNavigate()
    const [searchParams, setSearchParams] = useSearchParams()
    const tab = searchParams.get('tab') === 'packages' ? 'packages' : 'registries'

    const selectTab = (next: 'registries' | 'packages') => {
        const params = new URLSearchParams(searchParams)
        params.set('tab', next)
        setSearchParams(params, { replace: true })
    }

    return (
        <Layout>
            <Page
                title='Catalog'
                toolbar={<button className='app-button app-button--secondary' onClick={() => nav('/settings')}>Back to Settings</button>}
            >
                <div className={styles.tabs}>
                    <button
                        className={`${styles.tab}${tab === 'registries' ? ' ' + styles.tabActive : ''}`}
                        onClick={() => selectTab('registries')}
                    >Registries</button>
                    <button
                        className={`${styles.tab}${tab === 'packages' ? ' ' + styles.tabActive : ''}`}
                        onClick={() => selectTab('packages')}
                    >Packages</button>
                </div>

                {tab === 'registries' ? <RegistriesTab /> : <PackagesTab />}
            </Page>
        </Layout>
    )
}
