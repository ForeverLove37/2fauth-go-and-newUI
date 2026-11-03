import type {
  Account,
  ApiError,
  FrontendConfig,
  Group,
  ManagedUser,
  OtpPayload,
  PersonalAccessToken,
  Preference,
  ShareRecipient,
  User,
  WebAuthnCredential,
} from './types'

const API = '/api/v1'

/** Where the application is mounted, as told by the server ("" at the root). */
export const BASE =
  (typeof document !== 'undefined' && document.querySelector<HTMLMetaElement>('meta[name="2fauth-base"]')?.content) || ''

function cookie(name: string): string {
  const match = document.cookie.match(new RegExp('(^|;\\s*)' + name + '=([^;]*)'))
  return match ? decodeURIComponent(match[2]) : ''
}

function apiError(status: number, payload: unknown): ApiError {
  const body = (payload ?? {}) as {
    message?: string
    details?: string
    errors?: Record<string, string[]>
    needs_passphrase?: boolean
  }
  const message = body.message || `Request failed with status ${status}`
  const error = new Error(body.details ? `${message}: ${body.details}` : message) as ApiError
  error.status = status
  error.errors = body.errors
  error.needsPassphrase = body.needs_passphrase === true
  return error
}

interface RequestOptions {
  body?: unknown
  form?: FormData
  raw?: boolean
  retry?: boolean
}

async function request<T>(method: string, path: string, options: RequestOptions = {}): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' }
  if (options.body !== undefined) headers['Content-Type'] = 'application/json'
  const token = cookie('XSRF-TOKEN')
  if (token && method !== 'GET' && method !== 'HEAD') headers['X-XSRF-TOKEN'] = token

  const response = await fetch(BASE + path, {
    method,
    headers,
    credentials: 'same-origin',
    body: options.form ?? (options.body === undefined ? undefined : JSON.stringify(options.body)),
  })

  // Laravel regenerates the CSRF cookie on 419: refresh it once and replay.
  if (response.status === 419 && options.retry !== false) {
    await fetch(`${BASE}/refresh-csrf`, { credentials: 'same-origin', headers: { Accept: 'application/json' } })
    return request<T>(method, path, { ...options, retry: false })
  }

  if (response.status === 401 || response.status === 418) {
    window.dispatchEvent(new CustomEvent('2fauth:session-expired'))
  }
  if (response.status === 204) return undefined as T
  if (options.raw) return (await response.blob()) as unknown as T

  const text = await response.text()
  let payload: unknown = null
  if (text) {
    try {
      payload = JSON.parse(text)
    } catch {
      payload = { message: text }
    }
  }
  if (!response.ok) throw apiError(response.status, payload)
  return payload as T
}

const get = <T>(path: string) => request<T>('GET', path)
const post = <T>(path: string, body?: unknown) => request<T>('POST', path, { body })
const put = <T>(path: string, body?: unknown) => request<T>('PUT', path, { body })
const patch = <T>(path: string, body?: unknown) => request<T>('PATCH', path, { body })
const del = <T>(path: string) => request<T>('DELETE', path)

function fromEnvelope<T>(payload: { data: T }): T {
  return payload.data
}

export interface IconProvider {
  id: string
  label: string
  source: string
  template: string
  enabled: boolean
  order: number
  status?: string
}

export interface IconProvidersResponse {
  providers: IconProvider[]
  max_tries: number
  all?: IconProvider[]
}

export interface ImportResult {
  total: number
  imported: number
  duplicates: number
  invalid: number
  issues: { index: number; service: string; reason: string }[] | null
}

export interface MailSettings {
  mailer: string
  host: string
  port: number
  username: string
  password_set: boolean
  encryption: string
  verify_tls: boolean
  from_name: string
  from_address: string
  enabled: boolean
}

export const api = {
  config: () => get<FrontendConfig>(`${API}/frontend-config`),

  // authentication
  login: (email: string, password: string) =>
    post<{ message: string } & User>('/user/login', { email, password }),
  register: (name: string, email: string, password: string, confirmation: string) =>
    post<{ message: string } & User>('/user', { name, email, password, password_confirmation: confirmation }),
  logout: () => post<{ message: string }>('/user/logout'),
  passwordLost: (email: string) => post<{ message: string }>('/user/password/lost', { email }),
  passwordReset: (email: string, token: string, password: string) =>
    post<{ message: string }>('/user/password/reset', { email, token, password, password_confirmation: password }),
  sendEmailVerification: () => post<{ message: string }>('/user/email/verification'),
  me: async () => fromEnvelope(await get<{ data: User }>(`${API}/user`)),
  updateProfile: async (payload: { name: string; email: string; password: string }) =>
    fromEnvelope(await put<{ data: User }>('/user', payload)),
  updatePassword: (currentPassword: string, password: string) =>
    patch<{ message: string }>('/user/password', {
      currentPassword,
      password,
      password_confirmation: password,
    }),
  deleteProfile: (password: string) => request<void>('DELETE', '/user', { body: { password } }),

  // preferences
  preferences: () => get<Preference[]>(`${API}/user/preferences`),
  setPreference: (key: string, value: unknown) =>
    put<{ key: string; value: unknown }>(`${API}/user/preferences/${key}`, { value }),

  // accounts
  accounts: async (withOtp = true) =>
    fromEnvelope(
      await get<{ data: Account[] }>(`${API}/twofaccounts${withOtp ? '?withOtp=1' : ''}`),
    ),
  account: async (id: number) => fromEnvelope(await get<{ data: Account }>(`${API}/twofaccounts/${id}`)),
  createAccount: async (payload: unknown) =>
    fromEnvelope(await post<{ data: Account }>(`${API}/twofaccounts`, payload)),
  updateAccount: async (id: number, payload: unknown) =>
    fromEnvelope(await put<{ data: Account }>(`${API}/twofaccounts/${id}`, payload)),
  deleteAccount: (id: number) => del<void>(`${API}/twofaccounts/${id}`),
  deleteAccounts: (ids: number[]) => del<void>(`${API}/twofaccounts?ids=${ids.join(',')}`),
  withdrawAccounts: (ids: number[]) => patch<{ message: string }>(`${API}/twofaccounts/withdraw?ids=${ids.join(',')}`),
  reorderAccounts: (ids: number[]) => post<{ orderedIds: number[] }>(`${API}/twofaccounts/reorder`, { orderedIds: ids }),
  favorite: async (id: number, value: boolean) =>
    fromEnvelope(await patch<{ data: Account }>(`${API}/twofaccounts/${id}/favorite`, { is_favorite: value })),
  otp: (id: number) => get<OtpPayload>(`${API}/twofaccounts/${id}/otp`),
  otpFromUri: (uri: string) => post<OtpPayload>(`${API}/twofaccounts/otp`, { uri }),
  preview: async (uri: string, customOtp?: string) =>
    fromEnvelope(await post<{ data: Account }>(`${API}/twofaccounts/preview`, { uri, custom_otp: customOtp })),
  qrcode: (id: number) => get<{ qrcode: string }>(`${API}/twofaccounts/${id}/qrcode`),
  decodeQrCode: (file: File) => {
    const form = new FormData()
    form.append('qrcode', file)
    return request<{ data: string }>('POST', `${API}/qrcode/decode`, { form })
  },
  importPayload: (payload: string, passphrase?: string) =>
    post<ImportResult>(`${API}/twofaccounts/import`, { payload, ...(passphrase ? { passphrase } : {}) }),
  exportEncrypted: (ids: number[], passphrase: string) =>
    post<unknown>(`${API}/twofaccounts/export`, { ids: ids.join(','), passphrase }),
  exportAccounts: async (ids: number[], otpauth: boolean) =>
    fromEnvelope(
      await get<{ data: unknown }>(`${API}/twofaccounts/export?ids=${ids.join(',')}&otpauth=${otpauth ? 1 : 0}`),
    ),
  exportBlob: (ids: number[]) =>
    request<Blob>('GET', `${API}/twofaccounts/export?ids=${ids.join(',')}`, { raw: true }),
  count: () => get<{ owned: number; shared: number; total: number }>(`${API}/twofaccounts/count`),

  // groups
  groups: async () => fromEnvelope(await get<{ data: Group[] }>(`${API}/groups`)),
  createGroup: async (name: string, showInChips = true) =>
    fromEnvelope(await post<{ data: Group }>(`${API}/groups`, { name, show_in_chips: showInChips })),
  updateGroup: async (id: number, name: string, showInChips: boolean) =>
    fromEnvelope(await put<{ data: Group }>(`${API}/groups/${id}`, { name, show_in_chips: showInChips })),
  deleteGroup: (id: number) => del<void>(`${API}/groups/${id}`),
  assignGroup: (id: number, ids: number[]) => post<{ message: string }>(`${API}/groups/${id}/assign`, { ids }),
  withdrawFromGroup: (ids: number[]) => patch<{ message: string }>(`${API}/twofaccounts/withdraw?ids=${ids.join(',')}`),

  // sharing
  shares: (id: number) =>
    get<{ twofaccount_id: number; is_shared_with_all: boolean; specific_users?: ShareRecipient[] }>(
      `${API}/twofaccounts/${id}/shares`,
    ),
  recipients: async (id: number, filter = '') =>
    fromEnvelope(
      await get<{ data: ShareRecipient[] }>(
        `${API}/twofaccounts/${id}/recipients${filter ? `?filter[nameOrEmail]=${encodeURIComponent(filter)}` : ''}`,
      ),
    ),
  share: (id: number, userIds: number[]) =>
    post<{ twofaccount_id: number }>(`${API}/twofaccounts/${id}/shares`, { user_ids: userIds }),
  shareWithAll: (id: number) => post<unknown>(`${API}/twofaccounts/${id}/shares/all`),
  unshare: (id: number) => del<void>(`${API}/twofaccounts/${id}/shares`),

  // personal access tokens & credentials
  tokens: () => get<PersonalAccessToken[]>('/oauth/personal-access-tokens'),
  createToken: (name: string) =>
    post<{ accessToken: string; accessTokenId: string }>('/oauth/personal-access-tokens', { name }),
  deleteToken: (id: string) => del<void>(`/oauth/personal-access-tokens/${id}`),
  credentials: async () => fromEnvelope(await get<{ data: WebAuthnCredential[] }>('/webauthn/credentials')),
  deleteCredential: (id: string) => del<void>(`/webauthn/credentials/${id}`),

  // administration
  users: async () => {
    const payload = await get<{ info: ManagedUser[] }>(`${API}/users`)
    return payload.info
  },
  createUser: async (payload: { name: string; email: string; password: string; is_admin: boolean }) => {
    const response = await post<{ info: ManagedUser }>(`${API}/users`, payload)
    return response.info
  },
  deleteUser: (id: number) => del<void>(`${API}/users/${id}`),
  promoteUser: (id: number, isAdmin: boolean) =>
    patch<{ info: ManagedUser }>(`${API}/users/${id}/promote`, { is_admin: isAdmin }),
  resetUserPassword: (id: number) => patch<{ password: string }>(`${API}/users/${id}/password/reset`),
  revokeUserTokens: (id: number) => del<void>(`${API}/users/${id}/pats`),
  settings: async () => {
    const payload = await get<{ key: string; value: unknown }[]>(`${API}/settings`)
    return Object.fromEntries(payload.map((item) => [item.key, item.value])) as Record<string, unknown>
  },
  setSetting: (key: string, value: unknown) => put<{ key: string; value: unknown }>(`${API}/settings/${key}`, { value }),

  // icons
  iconPacks: () => get<{ packs: { name: string; label: string; url: string }[] }>(`${API}/icons/packs`),
  fetchIcon: (service: string) =>
    post<{ icon: string; url: string }>(`${API}/icons/default`, { service }),
  uploadIcon: (file: File) => {
    const form = new FormData()
    form.append('icon', file)
    return request<{ icon: string; url: string }>('POST', `${API}/icons`, { form })
  },
  iconProviders: () => get<IconProvidersResponse>(`${API}/icons/providers`),
  setIconProviders: (payload: { providers: string[]; max_tries: number }) =>
    put<IconProvidersResponse>(`${API}/icons/providers`, payload),
  iconUrl: (icon: string | null) => (icon ? `${BASE}/storage/icons/${encodeURIComponent(icon)}` : ''),

  // e-mail delivery
  mailSettings: () => get<MailSettings>(`${API}/mail/settings`),
  saveMailSettings: (payload: Partial<MailSettings> & { password?: string; clear_password?: boolean }) =>
    put<MailSettings>(`${API}/mail/settings`, payload),
  sendTestEmail: () => post<{ message: string }>('/system/test-email'),
}

export { API as apiBase }
