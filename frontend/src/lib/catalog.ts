import type { CatalogKind, CatalogPackage } from './api'

export type CatalogSortKey = 'starred' | 'title' | 'version'

export const CATALOG_ALL_PROVIDER = 'All'

export const CATALOG_CAPABILITY_FILTERS = [
  { id: 'openapi', label: 'REST/OpenAPI', terms: ['openapi', 'rest api', 'rest'] },
  { id: 'grpc', label: 'gRPC', terms: ['grpc', 'protobuf', 'proto'] },
  { id: 'soap', label: 'SOAP', terms: ['soap', 'wsdl', 'xml envelope'] },
  { id: 'graphql', label: 'GraphQL', terms: ['graphql'] },
  { id: 'async', label: 'Async', terms: ['async', 'event stream', 'event-driven'] },
  { id: 'kafka', label: 'Kafka', terms: ['kafka'] },
  { id: 'mqtt', label: 'MQTT', terms: ['mqtt'] },
  { id: 'websocket', label: 'WebSocket', terms: ['websocket', 'ws '] },
  { id: 'redis', label: 'Redis', terms: ['redis'] },
  { id: 'postgres', label: 'Postgres', terms: ['postgres'] },
  { id: 'wiremock', label: 'Wiremock', terms: ['wiremock'] },
  { id: 'mock-api', label: 'Mock API', terms: ['mock-api', 'mock api'] },
  { id: 'playwright', label: 'Playwright', terms: ['playwright'] },
  { id: 'vault', label: 'Vault', terms: ['vault'] },
  { id: 'prometheus', label: 'Prometheus', terms: ['prometheus'] },
] as const

export type CatalogCapabilityId = (typeof CATALOG_CAPABILITY_FILTERS)[number]['id']

interface CatalogFilterOptions {
  packages: CatalogPackage[]
  kind: CatalogKind
  provider: string
  capabilities: ReadonlySet<string>
  query: string
  sort: CatalogSortKey
}

export function filterCatalogPackages({
  packages,
  kind,
  provider,
  capabilities,
  query,
  sort,
}: CatalogFilterOptions) {
  const normalizedQuery = query.trim().toLowerCase()
  const filtered = packages.filter((item) => {
    if (item.kind !== kind) {
      return false
    }
    if (provider !== CATALOG_ALL_PROVIDER && item.provider !== provider) {
      return false
    }
    for (const capability of capabilities) {
      if (!matchesCatalogCapability(item, capability)) {
        return false
      }
    }
    if (!normalizedQuery) {
      return true
    }
    return catalogSearchText(item).includes(normalizedQuery)
  })

  return [...filtered].sort((left, right) => compareCatalogPackages(left, right, sort))
}

export function catalogProviders(packages: CatalogPackage[], kind: CatalogKind) {
  return [
    CATALOG_ALL_PROVIDER,
    ...Array.from(new Set(packages.filter((item) => item.kind === kind).map((item) => item.provider))),
  ]
}

export function catalogKindCounts(packages: CatalogPackage[]) {
  const counts = { suite: 0, stdlib: 0 } satisfies Record<CatalogKind, number>
  for (const item of packages) {
    counts[item.kind] += 1
  }
  return counts
}

export function catalogProviderCount(packages: CatalogPackage[], kind: CatalogKind, provider: string) {
  return packages.filter((item) => (
    item.kind === kind &&
    (provider === CATALOG_ALL_PROVIDER || item.provider === provider)
  )).length
}

export function availableCatalogCapabilities(packages: CatalogPackage[], kind: CatalogKind) {
  return CATALOG_CAPABILITY_FILTERS
    .map((capability) => ({
      ...capability,
      count: packages.filter((item) => item.kind === kind && matchesCatalogCapability(item, capability.id)).length,
    }))
    .filter((capability) => capability.count > 0)
}

export function matchesCatalogCapability(item: CatalogPackage, capabilityId: string) {
  const capability = CATALOG_CAPABILITY_FILTERS.find((candidate) => candidate.id === capabilityId)
  if (!capability) {
    return false
  }

  const haystack = catalogSearchText(item)
  return capability.terms.some((term) => haystack.includes(term))
}

function compareCatalogPackages(left: CatalogPackage, right: CatalogPackage, sort: CatalogSortKey) {
  if (sort === 'starred') {
    const starredDelta = Number(right.starred) - Number(left.starred)
    if (starredDelta !== 0) {
      return starredDelta
    }
    return compareText(left.title, right.title)
  }
  if (sort === 'title') {
    return compareText(left.title, right.title)
  }
  return compareText(right.version, left.version)
}

function catalogSearchText(item: CatalogPackage) {
  return [
    item.title,
    item.repository,
    item.owner,
    item.provider,
    item.description,
    item.modules.join(' '),
    item.tags.join(' '),
  ].join(' ').toLowerCase()
}

function compareText(left: string, right: string) {
  return left.localeCompare(right, undefined, { numeric: true, sensitivity: 'base' })
}
