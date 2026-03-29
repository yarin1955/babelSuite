import type { AuthResponse } from './types'

export class ApiError extends Error {
  status: number

  constructor(message: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

const API_BASE = (import.meta.env.VITE_API_URL ?? 'http://localhost:8090').replace(/\/$/, '')
const SESSION_KEY = 'babelsuite.session'
const GET_CACHE_TTL_MS = 5000

const inflightGetRequests = new Map<string, Promise<unknown>>()
const cachedGetResponses = new Map<string, { expiresAt: number; value: unknown }>()

export function getSession(): AuthResponse | null {
  const raw = window.localStorage.getItem(SESSION_KEY)
  if (!raw) {
    return null
  }

  try {
    return JSON.parse(raw) as AuthResponse
  } catch {
    window.localStorage.removeItem(SESSION_KEY)
    return null
  }
}

export function saveSession(session: AuthResponse) {
  clearRequestCache()
  window.localStorage.setItem(SESSION_KEY, JSON.stringify(session))
}

export function clearSession() {
  clearRequestCache()
  window.localStorage.removeItem(SESSION_KEY)
}

export function buildAuthenticatedStreamUrl(
  path: string,
  params: Record<string, string | number | boolean | undefined> = {},
) {
  const session = getSession()
  const url = new URL(`${API_BASE}${path}`)

  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === '') {
      continue
    }
    url.searchParams.set(key, String(value))
  }

  if (session?.token) {
    url.searchParams.set('token', session.token)
  }

  return url
}

export async function request<T>(path: string, init: RequestInit = {}) {
  const session = getSession()
  const method = (init.method ?? 'GET').toUpperCase()
  const cacheKey = method === httpMethodGet ? buildGetCacheKey(path, session?.token ?? '') : ''

  if (method === httpMethodGet) {
    const cached = cachedGetResponses.get(cacheKey)
    if (cached && cached.expiresAt > Date.now()) {
      return clonePayload(cached.value as T)
    }

    const inflight = inflightGetRequests.get(cacheKey)
    if (inflight) {
      return clonePayload((await inflight) as T)
    }
  }

  const runRequest = async () => {
    const response = await fetch(`${API_BASE}${path}`, {
      ...init,
      headers: {
        'Content-Type': 'application/json',
        ...(session?.token ? { Authorization: `Bearer ${session.token}` } : {}),
        ...(init.headers ?? {}),
      },
    })

    const text = await response.text()
    const payload = parseResponsePayload(response, text)

    if (!response.ok) {
      const message = extractErrorMessage(path, response, payload)
      throw new ApiError(message, response.status)
    }

    if (typeof payload === 'string') {
      throw new ApiError(extractUnexpectedSuccessMessage(path, response, payload), response.status)
    }

    return payload
  }

  if (method === httpMethodGet) {
    const inflight = runRequest()
    inflightGetRequests.set(cacheKey, inflight)
    try {
      const payload = await inflight
      cachedGetResponses.set(cacheKey, {
        expiresAt: Date.now() + GET_CACHE_TTL_MS,
        value: clonePayload(payload),
      })
      return clonePayload(payload as T)
    } finally {
      inflightGetRequests.delete(cacheKey)
    }
  }

  const payload = await runRequest()
  clearRequestCache()
  return payload as T
}

const httpMethodGet = 'GET'

function buildGetCacheKey(path: string, token: string) {
  return `${token}::${path}`
}

function clearRequestCache() {
  inflightGetRequests.clear()
  cachedGetResponses.clear()
}

function clonePayload<T>(value: T): T {
  if (value === null || value === undefined) {
    return value
  }

  if (typeof structuredClone === 'function') {
    return structuredClone(value)
  }

  return JSON.parse(JSON.stringify(value)) as T
}

function parseResponsePayload(response: Response, text: string): unknown {
  const trimmed = text.trim()
  if (!trimmed) {
    return null
  }

  const contentType = response.headers.get('content-type')?.toLowerCase() ?? ''
  const looksJson = contentType.includes('application/json') || trimmed.startsWith('{') || trimmed.startsWith('[')
  if (!looksJson) {
    return trimmed
  }

  try {
    return JSON.parse(trimmed) as unknown
  } catch {
    return trimmed
  }
}

function extractErrorMessage(
  path: string,
  response: Response,
  payload: unknown,
) {
  if (
    payload &&
    typeof payload === 'object' &&
    'error' in payload &&
    typeof payload.error === 'string' &&
    payload.error
  ) {
    return payload.error
  }

  if (typeof payload === 'string') {
    const normalized = normalizePayloadSnippet(payload)
    if (response.status === 404 && normalized.toLowerCase().includes('page not found')) {
      return `The backend at ${API_BASE} does not expose ${path}. You are likely connected to an older BabelSuite server.`
    }

    if (normalized.startsWith('<!doctype html') || normalized.startsWith('<html')) {
      return `The server returned HTML instead of JSON for ${path}. Check VITE_API_URL and make sure the BabelSuite API is running.`
    }

    return `The backend returned a non-JSON ${response.status} response for ${path}: ${truncatePayload(normalized)}`
  }

  return `Request to ${path} failed with status ${response.status}.`
}

function extractUnexpectedSuccessMessage(path: string, response: Response, payload: string) {
  const normalized = normalizePayloadSnippet(payload)
  if (normalized.startsWith('<!doctype html') || normalized.startsWith('<html')) {
    return `Expected JSON from ${path}, but the server returned HTML. Check VITE_API_URL and make sure the API points to the backend, not the frontend dev server.`
  }

  return `Expected JSON from ${path}, but the server returned ${response.headers.get('content-type') || 'text'}: ${truncatePayload(normalized)}`
}

function normalizePayloadSnippet(value: string) {
  return value.replace(/\s+/g, ' ').trim()
}

function truncatePayload(value: string) {
  return value.length > 160 ? `${value.slice(0, 157)}...` : value
}
