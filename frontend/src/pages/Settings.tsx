import { useMemo } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  FaArrowRight, FaBoxOpen, FaGear, FaRobot, FaSliders, FaUser,
} from 'react-icons/fa6'
import Layout from '../components/Layout'
import Page from '../components/Page'
import styles from './Settings.module.css'

interface StoredUser {
  name?: string
  username?: string
  email?: string
  is_admin?: boolean
}

function currentUser(): StoredUser | null {
  try {
    const raw = localStorage.getItem('user')
    return raw ? JSON.parse(raw) as StoredUser : null
  } catch {
    return null
  }
}

export default function Settings() {
  const nav = useNavigate()
  const user = currentUser()
  const isAdmin = user?.is_admin === true

  const managerItems = useMemo(() => [
    {
      title: 'Agent Pools',
      description: 'Configure worker tokens, runtime classes, and named execution targets.',
      path: '/settings/agents',
      icon: <FaRobot />,
    },
    {
      title: 'Catalog Sources',
      description: 'Connect registries and sync suite packages into the workspace catalog.',
      path: '/settings/catalog?tab=registries',
      icon: <FaGear />,
    },
    {
      title: 'Suite Availability',
      description: 'Enable or disable the packages that appear on the Suites page for users.',
      path: '/settings/catalog?tab=packages',
      icon: <FaBoxOpen />,
    },
    {
      title: 'Shared Profiles',
      description: 'Create and edit reusable YAML or JSON environment profiles for suite runs.',
      path: '/settings/profiles',
      icon: <FaSliders />,
    },
  ], [])

  return (
    <Layout>
      <Page title='Settings'>
        <div className={styles.shell}>
          {isAdmin ? (
            <section className={styles.section}>
              <div className={styles.sectionTop}>
                <div className={styles.eyebrow}>Manager controls</div>
                <h2>Workspace configuration</h2>
                <p>Open the control surfaces admins use to manage catalogs, agents, registries, and shared profiles.</p>
              </div>
              <div className={styles.redirectList}>
                {managerItems.map(item => (
                  <button key={item.path} className={styles.redirectPanel} onClick={() => nav(item.path)}>
                    <div className={styles.redirectIcon}>{item.icon}</div>
                    <div className={styles.redirectContent}>
                      <div className={styles.redirectTitle}>{item.title}</div>
                      <div className={styles.redirectDescription}>{item.description}</div>
                    </div>
                    <div className={styles.redirectArrow}><FaArrowRight /></div>
                  </button>
                ))}
              </div>
            </section>
          ) : null}

          <section className={styles.section}>
            <div className={styles.sectionTop}>
              <div className={styles.eyebrow}>Account</div>
              <h2>Your access</h2>
              <p>Review the current signed-in identity and workspace role.</p>
            </div>
            <div className={styles.accountCard}>
              <div className={styles.accountHeader}>
                <div className={styles.accountAvatar}><FaUser /></div>
                <div>
                  <strong>{user?.name || 'Unknown user'}</strong>
                  <div className={styles.accountRole}>{isAdmin ? 'Manager' : 'Member'}</div>
                </div>
              </div>

              <div className={styles.detailGrid}>
                <div className={styles.detailRow}>
                  <span>Name</span>
                  <strong>{user?.name || 'Unknown'}</strong>
                </div>
                <div className={styles.detailRow}>
                  <span>Username</span>
                  <strong>{user?.username ? `@${user.username}` : 'Unknown'}</strong>
                </div>
                <div className={styles.detailRow}>
                  <span>Email</span>
                  <strong>{user?.email || 'Unknown'}</strong>
                </div>
                <div className={styles.detailRow}>
                  <span>Role</span>
                  <strong>{isAdmin ? 'Manager' : 'Member'}</strong>
                </div>
              </div>

              {!isAdmin && (
                <div className={styles.accountActions}>
                  <button className='app-button app-button--secondary' onClick={() => nav('/profiles')}>
                    <FaSliders />
                    Open profiles
                  </button>
                </div>
              )}
            </div>
          </section>
        </div>
      </Page>
    </Layout>
  )
}
