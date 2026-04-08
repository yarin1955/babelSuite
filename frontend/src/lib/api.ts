import { request } from './api/client'
import type { AuthResponse } from './api/types'

export { ApiError, clearSession, getSession, saveSession } from './api/client'
export type { AuthResponse, User, Workspace } from './api/types'

export interface APISIXSidecarConfig {
  image: string
  configMountPath: string
  listenPort: number
  adminPort: number
  capabilities: string[]
}

export interface ExecutionAgent {
  agentId: string
  name: string
  type: 'local' | 'remote-agent' | 'remote-docker' | 'kubernetes'
  description: string
  enabled: boolean
  default: boolean
  status: string
  registeredAt?: string
  lastHeartbeatAt?: string
  routingTags: string[]
  runtimeCapabilities: string[]
  dockerSocket: string
  hostUrl: string
  tlsCert: string
  tlsKey: string
  kubeconfigPath: string
  targetNamespace: string
  serviceAccountToken: string
  apisixSidecar: APISIXSidecarConfig
}

export interface RuntimeAgent {
  agentId: string
  name: string
  hostUrl: string
  status: string
  capabilities: string[]
  registeredAt: string
  lastHeartbeatAt: string
}

export interface OCIRegistry {
  registryId: string
  name: string
  provider: string
  registryUrl: string
  username: string
  secret: string
  repositoryScope: string
  region: string
  syncStatus: string
  lastSyncedAt?: string
}

export interface GlobalOverride {
  key: string
  value: string
  description: string
  sensitive: boolean
}

export interface SecretsConfig {
  provider: 'none' | 'vault' | 'aws-secrets-manager'
  vaultAddress: string
  vaultNamespace: string
  vaultRole: string
  awsRegion: string
  secretPrefix: string
  globalOverrides: GlobalOverride[]
}

export interface PlatformSettings {
  mode: string
  description: string
  agents: ExecutionAgent[]
  registries: OCIRegistry[]
  secrets: SecretsConfig
  updatedAt: string
}

export interface SandboxUsage {
  cpuPercent: number
  memoryBytes: number
  memoryLimitBytes: number
  memoryPercent: number
}

export interface SandboxContainer {
  id: string
  name: string
  image: string
  state: string
  status: string
  ports: string[]
  startedAt?: string
  exitCode?: number
  usage: SandboxUsage
}

export interface SandboxNetwork {
  id: string
  name: string
  driver: string
  scope: string
}

export interface SandboxVolume {
  name: string
  driver: string
  mountpoint: string
}

export interface SandboxRecord {
  sandboxId: string
  runId: string
  suite: string
  owner: string
  profile: string
  status: string
  summary: string
  startedAt?: string
  lastHeartbeatAt?: string
  orchestratorPid?: number
  orchestratorState: string
  isZombie: boolean
  canReap: boolean
  resourceUsage: SandboxUsage
  containers: SandboxContainer[]
  networks: SandboxNetwork[]
  volumes: SandboxVolume[]
  warnings: string[]
}

export interface SandboxesResponse {
  dockerAvailable: boolean
  updatedAt: string
  summary: {
    activeSandboxes: number
    zombieSandboxes: number
    containers: number
    networks: number
    volumes: number
    totalCpuPercent: number
    totalMemoryBytes: number
  }
  sandboxes: SandboxRecord[]
  warnings: string[]
}

export interface SandboxStreamEvent {
  id: number
  reason: string
  snapshot: SandboxesResponse
}

export interface ReapResult {
  scope: string
  target: string
  removedContainers: number
  removedNetworks: number
  removedVolumes: number
  warnings: string[]
}

export type CatalogKind = 'suite' | 'stdlib'
export type PackageStatus = 'Official' | 'Verified'

export interface CatalogPackage {
  id: string
  kind: CatalogKind
  title: string
  repository: string
  owner: string
  provider: string
  version: string
  tags: string[]
  description: string
  modules: string[]
  status: PackageStatus
  score: number
  pullCommand: string
  forkCommand: string
  inspectable: boolean
  starred: boolean
}

export interface ExecutionProfileOption {
  fileName: string
  label: string
  description: string
  default: boolean
}

export interface ExecutionLaunchSuite {
  id: string
  title: string
  repository: string
  description: string
  provider: string
  status: string
  profiles: ExecutionProfileOption[]
  backends: ExecutionBackendOption[]
}

export interface ExecutionBackendOption {
  id: string
  label: string
  kind: 'local' | 'kubernetes' | 'remote' | string
  description: string
  default: boolean
  available: boolean
}

export interface ExecutionSummary {
  id: string
  suiteId: string
  suiteTitle: string
  profile: string
  backendId: string
  backend: string
  trigger: string
  status: 'Booting' | 'Healthy' | 'Failed'
  duration: string
  startedAt: string
}

export interface ExecutionOverviewStep {
  id: string
  name: string
  kind: string
  status: 'pending' | 'running' | 'healthy' | 'failed'
  dependsOn: string[]
  level: number
}

export interface ExecutionOverviewItem {
  id: string
  suiteId: string
  suiteTitle: string
  profile: string
  backendId: string
  backend: string
  trigger: string
  status: 'Booting' | 'Healthy' | 'Failed'
  duration: string
  startedAt: string
  updatedAt: string
  totalSteps: number
  runningSteps: number
  healthySteps: number
  failedSteps: number
  pendingSteps: number
  progressRatio: number
  steps: ExecutionOverviewStep[]
}

export interface ExecutionOverview {
  updatedAt: string
  summary: {
    totalExecutions: number
    bootingExecutions: number
    healthyExecutions: number
    failedExecutions: number
    totalSteps: number
    runningSteps: number
    healthySteps: number
    failedSteps: number
    pendingSteps: number
  }
  executions: ExecutionOverviewItem[]
}

export interface ExecutionEventRecord {
  id: string
  source: string
  timestamp: string
  text: string
  status: 'pending' | 'running' | 'healthy' | 'failed'
  level: 'info' | 'warn' | 'error'
}

export interface ExecutionRecord {
  id: string
  suite: {
    id: string
    title: string
    repository: string
    suiteStar: string
    profiles: ExecutionProfileOption[]
    folders: SuiteFolderEntry[]
    sourceFiles: SuiteSourceFile[]
    topology: SuiteTopologyNode[]
    apiSurfaces: SuiteApiSurface[]
  }
  profile: string
  backendId: string
  backend: string
  trigger: string
  status: 'Booting' | 'Healthy' | 'Failed'
  duration: string
  startedAt: string
  updatedAt: string
  author: string
  commit: string
  branch: string
  message: string
  events: ExecutionEventRecord[]
}

export interface SuiteFolderEntry {
  name: string
  role: 'Core' | 'Extension'
  description: string
  files: string[]
}

export interface SuiteSourceFile {
  path: string
  language: string
  content: string
}

export interface SuiteTopologyNode {
  id: string
  name: string
  kind: string
  dependsOn: string[]
  level: number
}

export interface SuiteExchangeExample {
  name: string
  sourceArtifact: string
  dispatchCriteria: string
  requestHeaders: Array<{ name: string; value: string }>
  requestBody: string
  responseStatus: string
  responseMediaType: string
  responseHeaders: Array<{ name: string; value: string }>
  responseBody: string
}

export interface SuiteParameterConstraint {
  name: string
  in: 'path' | 'query' | 'header'
  required: boolean
  recopy: boolean
  mustMatchRegexp?: string
}

export interface SuiteMockFallback {
  mode: string
  exampleName?: string
  proxyUrl?: string
  status?: string
  mediaType?: string
  body?: string
  headers?: Array<{ name: string; value: string }>
}

export interface SuiteMockStateTransition {
  onExample: string
  mutationKeyTemplate?: string
  set?: Record<string, string>
  delete?: string[]
  increment?: Record<string, number>
}

export interface SuiteMockState {
  lookupKeyTemplate?: string
  mutationKeyTemplate?: string
  defaults?: Record<string, string>
  transitions?: SuiteMockStateTransition[]
}

export interface SuiteMockMetadata {
  adapter: 'rest' | 'grpc' | 'async'
  dispatcher: string
  dispatcherRules?: string
  delayMillis?: number
  parameterConstraints?: SuiteParameterConstraint[]
  fallback?: SuiteMockFallback
  state?: SuiteMockState
  metadataPath?: string
  resolverUrl?: string
  runtimeUrl?: string
}

export interface SuiteApiOperation {
  id: string
  method: string
  name: string
  summary: string
  contractPath: string
  mockPath: string
  mockUrl: string
  curlCommand: string
  dispatcher: string
  mockMetadata: SuiteMockMetadata
  exchanges: SuiteExchangeExample[]
}

export interface SuiteApiSurface {
  id: string
  title: string
  protocol: 'REST' | 'gRPC' | 'Async' | 'SOAP' | 'GraphQL' | 'Kafka' | 'MQTT' | 'WebSocket' | 'SSE' | 'AMQP' | 'NATS' | 'TCP' | 'UDP' | 'Webhook'
  mockHost: string
  description: string
  operations: SuiteApiOperation[]
}

export interface SuiteDefinition {
  id: string
  title: string
  repository: string
  owner: string
  provider: string
  version: string
  tags: string[]
  description: string
  modules: string[]
  status: PackageStatus
  score: number
  pullCommand: string
  forkCommand: string
  suiteStar: string
  profiles: ExecutionProfileOption[]
  folders: SuiteFolderEntry[]
  sourceFiles: SuiteSourceFile[]
  topology: SuiteTopologyNode[]
  topologyError?: string
  contracts: string[]
  apiSurfaces: SuiteApiSurface[]
}

export interface ProfileSecretReference {
  key: string
  provider: string
  ref: string
}

export interface ProfileRecord {
  id: string
  name: string
  fileName: string
  description: string
  scope: string
  yaml: string
  secretRefs: ProfileSecretReference[]
  default: boolean
  extendsId?: string
  launchable: boolean
  updatedAt: string
}

export interface ProfileSuiteSummary {
  id: string
  title: string
  description: string
  repository: string
  profileCount: number
  launchableCount: number
  defaultProfileFileName: string
}

export interface SuiteProfilesResponse {
  suiteId: string
  suiteTitle: string
  suiteDescription: string
  repository: string
  defaultProfileId: string
  defaultProfileFileName: string
  profiles: ProfileRecord[]
}

export interface ExecutionStreamEvent {
  id: number
  executionId: string
  executionStatus: 'Booting' | 'Healthy' | 'Failed'
  duration: string
  updatedAt: string
  event: ExecutionEventRecord
}

export interface ExecutionLogLine {
  source: string
  timestamp: string
  level: 'info' | 'warn' | 'error'
  text: string
}

export interface ExecutionLogStreamRecord {
  id: number
  executionId: string
  line: ExecutionLogLine
}

export interface SSOProvider {
  providerId: string
  name: string
  buttonLabel: string
  startUrl?: string
  enabled: boolean
  hint?: string
}

export interface AuthConfig {
  passwordAuthEnabled: boolean
  signUpEnabled: boolean
  providers: SSOProvider[]
}

export const fallbackSSOProviders: SSOProvider[] = [
  {
    providerId: 'oidc',
    name: 'Single Sign-On',
    buttonLabel: 'Continue with Single Sign-On',
    enabled: false,
    hint: 'Configure the backend OIDC settings to enable single sign-on.',
  },
]

export const fallbackAuthConfig: AuthConfig = {
  passwordAuthEnabled: true,
  signUpEnabled: true,
  providers: fallbackSSOProviders,
}

export async function signIn(payload: { email: string; password: string }) {
  return request<AuthResponse>('/api/v1/auth/sign-in', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export async function signUp(payload: { fullName: string; email: string; password: string }) {
  return request<AuthResponse>('/api/v1/auth/sign-up', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export async function listSSOProviders() {
  const response = await request<{ providers: SSOProvider[] }>('/api/v1/auth/sso/providers')
  return response.providers
}

export async function getAuthConfig() {
  return request<AuthConfig>('/api/v1/auth/config')
}

export async function resolveSessionFromToken(token: string) {
  return request<AuthResponse>('/api/v1/auth/me', {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })
}

export async function getPlatformSettings() {
  return request<PlatformSettings>('/api/v1/platform-settings')
}

export async function updatePlatformSettings(settings: PlatformSettings) {
  return request<PlatformSettings>('/api/v1/platform-settings', {
    method: 'PUT',
    body: JSON.stringify(settings),
  })
}

export async function listAgents() {
  const response = await request<{ agents: RuntimeAgent[] }>('/api/v1/agents')
  return response.agents
}

export async function syncRegistry(registryId: string) {
  return request<PlatformSettings>(`/api/v1/platform-settings/registries/${encodeURIComponent(registryId)}/sync`, {
    method: 'POST',
  })
}

export async function listCatalogPackages() {
  const response = await request<{ packages: CatalogPackage[] }>('/api/v1/catalog/packages')
  return response.packages
}

export async function listCatalogFavorites() {
  const response = await request<{ packageIds: string[] }>('/api/v1/catalog/favorites')
  return response.packageIds
}

export async function addCatalogFavorite(packageId: string) {
  return request<{ packageId: string; starred: boolean }>(`/api/v1/catalog/favorites/${encodeURIComponent(packageId)}`, {
    method: 'POST',
  })
}

export async function removeCatalogFavorite(packageId: string) {
  return request<{ packageId: string; starred: boolean }>(`/api/v1/catalog/favorites/${encodeURIComponent(packageId)}`, {
    method: 'DELETE',
  })
}

export async function getSuite(suiteId: string) {
  return request<SuiteDefinition>(`/api/v1/suites/${encodeURIComponent(suiteId)}`)
}

export async function listProfileSuites() {
  const response = await request<{ suites: ProfileSuiteSummary[] }>('/api/v1/profiles/suites')
  return response.suites
}

export async function getSuiteProfiles(suiteId: string) {
  return request<SuiteProfilesResponse>(`/api/v1/profiles/suites/${encodeURIComponent(suiteId)}`)
}

export async function createSuiteProfile(
  suiteId: string,
  payload: Omit<ProfileRecord, 'id' | 'launchable' | 'updatedAt'>,
) {
  return request<SuiteProfilesResponse>(`/api/v1/profiles/suites/${encodeURIComponent(suiteId)}`, {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export async function updateSuiteProfile(
  suiteId: string,
  profileId: string,
  payload: Omit<ProfileRecord, 'id' | 'launchable' | 'updatedAt'>,
) {
  return request<SuiteProfilesResponse>(`/api/v1/profiles/suites/${encodeURIComponent(suiteId)}/${encodeURIComponent(profileId)}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

export async function deleteSuiteProfile(suiteId: string, profileId: string) {
  return request<SuiteProfilesResponse>(`/api/v1/profiles/suites/${encodeURIComponent(suiteId)}/${encodeURIComponent(profileId)}`, {
    method: 'DELETE',
  })
}

export async function setDefaultSuiteProfile(suiteId: string, profileId: string) {
  return request<SuiteProfilesResponse>(`/api/v1/profiles/suites/${encodeURIComponent(suiteId)}/${encodeURIComponent(profileId)}/default`, {
    method: 'POST',
  })
}

export async function getSandboxes() {
  return request<SandboxesResponse>('/api/v1/sandboxes')
}

export async function reapSandbox(sandboxId: string) {
  return request<ReapResult>(`/api/v1/sandboxes/${encodeURIComponent(sandboxId)}/reap`, {
    method: 'POST',
  })
}

export async function reapAllSandboxes() {
  return request<ReapResult>('/api/v1/sandboxes/reap-all', {
    method: 'POST',
  })
}

export async function listExecutionLaunchSuites() {
  const response = await request<{ suites: ExecutionLaunchSuite[] }>('/api/v1/executions/launch-suites')
  return response.suites
}

export async function listExecutions() {
  const response = await request<{ executions: ExecutionSummary[] }>('/api/v1/executions')
  return response.executions
}

export async function getExecutionOverview() {
  return request<ExecutionOverview>('/api/v1/executions/overview')
}

export async function createExecution(payload: { suiteId: string; profile: string; backend?: string }) {
  return request<ExecutionSummary>('/api/v1/executions', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export async function getExecution(executionId: string) {
  return request<ExecutionRecord>(`/api/v1/executions/${encodeURIComponent(executionId)}`)
}

export {
  openExecutionEventStream as streamExecutionEvents,
  openExecutionLogStream as streamExecutionLogs,
  openSandboxEventStream as streamSandboxEvents,
} from './stream/events'
