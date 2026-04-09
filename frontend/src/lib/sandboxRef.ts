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

  const commandMatch = value.match(/(?:^|\s)(?:babelctl|babelsuite)\s+run\s+(\S+)/i)
  if (commandMatch?.[1]) {
    value = commandMatch[1].trim()
  }

  value = value.replace(/^oci:\/\//i, '')
  value = value.replace(/,+$/, '')
  return value
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
