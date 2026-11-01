export interface SsoDiscovery {
  issuer: string
  authorization_endpoint: string
  token_endpoint: string
  userinfo_endpoint?: string
  end_session_endpoint?: string
}

export interface SsoConfig {
  enabled: boolean
  provider: string
  label: string
  auto_redirect: boolean
  auto_register: boolean
  issuer: string
  redirect_uri: string
  start_url: string
  discovery: SsoDiscovery | null
  error: string | null
}

export interface SsoSettings {
  enabled: boolean
  issuer: string
  client_id: string
  client_secret_set: boolean
  scopes: string
  label: string
  auto_register: boolean
  auto_redirect: boolean
  redirect_uri: string
}

export interface SsoPreset {
  id: string
  name: string
  issuer: string
  scopes: string
  hint: string
}

export interface KeycloakExport {
  filename: string
  format: string
  content: string
  secret_generated: boolean
  notice: string
}

async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' }
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  const match = document.cookie.match(/(^|;\s*)XSRF-TOKEN=([^;]*)/)
  if (match && method !== 'GET') headers['X-XSRF-TOKEN'] = decodeURIComponent(match[2])

  let response = await fetch(path, {
    method,
    headers,
    credentials: 'same-origin',
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (response.status === 419) {
    await fetch('/refresh-csrf', { credentials: 'same-origin', headers: { Accept: 'application/json' } })
    const retryToken = document.cookie.match(/(^|;\s*)XSRF-TOKEN=([^;]*)/)
    if (retryToken) headers['X-XSRF-TOKEN'] = decodeURIComponent(retryToken[2])
    response = await fetch(path, {
      method,
      headers,
      credentials: 'same-origin',
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  }

  const text = await response.text()
  const payload = text ? JSON.parse(text) : null
  if (!response.ok) {
    const error = new Error(payload?.message || payload?.error || `请求失败（${response.status}）`)
    throw error
  }
  return payload as T
}

export const sso = {
  config: () => call<SsoConfig>('GET', '/api/v1/sso/config'),

  /** Returns the IdP authorize URL; the caller navigates to it. */
  start: async (): Promise<string> => {
    const response = await call<{ url: string }>('GET', '/socialite/redirect/openid?format=json')
    return response.url
  },

  settings: () => call<SsoSettings>('GET', '/api/v1/sso/settings'),
  saveSettings: (payload: Partial<SsoSettings> & { client_secret?: string; clear_secret?: boolean }) =>
    call<SsoSettings>('PUT', '/api/v1/sso/settings', payload),
  test: () => call<{ ok: boolean; discovery?: SsoDiscovery; error?: string }>('POST', '/api/v1/sso/test'),
  presets: () => call<SsoPreset[]>('GET', '/api/v1/sso/presets'),
  keycloak: (format: 'client' | 'partial' | 'cli', params: { realm?: string; client_id?: string; base_url?: string } = {}) => {
    const query = new URLSearchParams({ format })
    if (params.realm) query.set('realm', params.realm)
    if (params.client_id) query.set('client_id', params.client_id)
    if (params.base_url) query.set('base_url', params.base_url)
    return call<KeycloakExport>('GET', `/api/v1/sso/keycloak-client?${query.toString()}`)
  },
}
