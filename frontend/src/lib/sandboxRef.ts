import type { CatalogPackage, ExecutionLaunchSuite } from './api'

export const SANDBOX_SUITE_REF_MIME = 'application/x-babelsuite-suite-ref'

export function buildSuiteDropRef(repository: string, version: string) {
  const trimmedRepository = strings(repository)
  const trimmedVersion = strings(version)
  if (trimmedRepository === '') {
    return ''
  }
  if (trimmedVersion === '' || trimmedVersion.toLowerCase() === 'latest') {
    return trimmedRepository
  }
  return `${trimmedRepository}:${trimmedVersion}`
}

export function extractSuiteRef(input: string) {
  const firstLine = input
    .split(/\r?\n/)
    .map((line) => line.trim())
    .find((line) => line !== '')

  let value = strings(firstLine)
  if (value === '') {
    return ''
  }

  value = value.replace(/^['"]+|['"]+$/g, '')

  const exampleSuiteID = suiteIDFromExamplesPath(value)
  if (exampleSuiteID !== '') {
    return exampleSuiteID
  }

  const commandMatch = value.match(/(?:^|\s)(?:babelctl|babelsuite)\s+run\s+(\S+)/i)
  if (commandMatch?.[1]) {
    value = commandMatch[1].trim()
  }

  value = value.replace(/^oci:\/\//i, '')
  value = value.replace(/,+$/, '')
  return value
}

export function extractSuiteRefs(input: string) {
  const refs = new Set<string>()
  const primary = extractSuiteRef(input)
  if (primary !== '') {
    refs.add(primary)
  }

  for (const candidate of inferredSuiteRefsFromPath(input)) {
    if (candidate !== '') {
      refs.add(candidate)
    }
  }

  return Array.from(refs)
}

export function normalizeSuiteLookupKey(input: string) {
  const value = extractSuiteRef(input).replace(/\/+$/, '')
  if (value === '') {
    return ''
  }

  const digestIndex = value.indexOf('@')
  if (digestIndex >= 0) {
    return value.slice(0, digestIndex)
  }

  const lastSlash = value.lastIndexOf('/')
  const lastColon = value.lastIndexOf(':')
  if (lastColon > lastSlash) {
    return value.slice(0, lastColon)
  }

  return value
}

export function repositoryPath(input: string) {
  const value = normalizeSuiteLookupKey(input)
  if (value === '') {
    return ''
  }

  const firstSlash = value.indexOf('/')
  if (firstSlash < 0) {
    return value
  }

  const head = value.slice(0, firstSlash)
  if (head === 'localhost' || head.includes('.') || head.includes(':')) {
    return value.slice(firstSlash + 1)
  }

  return value
}

export function resolveLaunchSuiteRef(launchSuites: ExecutionLaunchSuite[], input: string) {
  const normalized = normalizeSuiteLookupKey(input)
  const normalizedPath = repositoryPath(input)
  if (normalized === '') {
    return null
  }

  for (const suite of launchSuites) {
    const suiteRepository = normalizeSuiteLookupKey(suite.repository)
    const suitePath = repositoryPath(suite.repository)
    if (
      strings(suite.id) === normalized ||
      suiteRepository === normalized ||
      suitePath === normalizedPath
    ) {
      return suite
    }
  }

  return null
}

export function resolveCatalogPackageRef(items: CatalogPackage[], input: string) {
  const normalized = normalizeSuiteLookupKey(input)
  const normalizedPath = repositoryPath(input)
  if (normalized === '') {
    return null
  }

  for (const item of items) {
    if (item.kind !== 'suite') {
      continue
    }
    const itemRepository = normalizeSuiteLookupKey(item.repository)
    const itemPath = repositoryPath(item.repository)
    if (item.id === normalized || itemRepository === normalized || itemPath === normalizedPath) {
      return item
    }
  }

  return null
}

function strings(value?: string | null) {
  return (value ?? '').trim()
}

function suiteIDFromExamplesPath(input: string) {
  const decoded = decodeLocalDropValue(input)
  if (decoded === '') {
    return ''
  }

  const normalized = decoded
    .replace(/[?#].*$/, '')
    .replace(/\\/g, '/')
    .replace(/\/+/g, '/')

  const match = normalized.match(/(?:^|\/)examples\/oci-suites\/([^/]+)/i)
  return strings(match?.[1])
}

function inferredSuiteRefsFromPath(input: string) {
  const decoded = decodeLocalDropValue(input)
  if (decoded === '') {
    return []
  }

  const normalized = decoded
    .replace(/[?#].*$/, '')
    .replace(/\\/g, '/')
    .replace(/\/+/g, '/')
    .replace(/^\/+/, '')

  if (normalized === '') {
    return []
  }

  const refs = new Set<string>()
  const segments = normalized.split('/').filter(Boolean)
  if (segments.length === 0) {
    return []
  }

  const exampleSuiteID = suiteIDFromExamplesPath(decoded)
  if (exampleSuiteID !== '') {
    refs.add(exampleSuiteID)
  }

  const lastSegment = segments[segments.length - 1]
  const parentSegment = segments.length > 1 ? segments[segments.length - 2] : ''
  if (/^suite\.star$/i.test(lastSegment) && parentSegment !== '') {
    refs.add(parentSegment)
  }

  const profilesIndex = segments.findIndex((segment) => segment.toLowerCase() === 'profiles')
  if (profilesIndex > 0) {
    refs.add(segments[profilesIndex - 1])
  }

  if (segments.length === 1 && !segments[0].includes('.')) {
    refs.add(segments[0])
  }

  return Array.from(refs).map(strings).filter(Boolean)
}

function decodeLocalDropValue(input: string) {
  const value = strings(input)
  if (value === '') {
    return ''
  }

  if (!/^file:\/\//i.test(value)) {
    return value
  }

  try {
    const url = new URL(value)
    let pathname = decodeURIComponent(url.pathname)
    if (/^\/[a-zA-Z]:/.test(pathname)) {
      pathname = pathname.slice(1)
    }
    if (url.host && url.host !== 'localhost') {
      return `//${url.host}${pathname}`
    }
    return pathname
  } catch {
    return value.replace(/^file:\/+/i, '')
  }
}
