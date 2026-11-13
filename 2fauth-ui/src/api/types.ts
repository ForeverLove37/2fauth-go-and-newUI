export type OtpType = 'totp' | 'hotp' | 'steamtotp'

export interface OtpPayload {
  password: string
  otp_type: string
  generated_at?: number
  next_password?: string
  period?: number
  counter?: number
}

export interface Account {
  id: number
  group_id: number | null
  otp_type: OtpType
  account: string | null
  service: string | null
  icon: string | null
  secret?: string
  digits: number
  algorithm: string
  period: number | null
  counter: number | null
  is_favorite: boolean
  is_borrowed?: boolean
  borrowed_by?: string
  is_shared?: boolean
  is_shared_with_all?: boolean
  /** the phone app keeps a copy of the secret ("normal" code); otherwise the phone only gets codes */
  phone_sync?: boolean
  otp?: OtpPayload
}

export interface Group {
  id: number
  name: string
  show_in_chips: boolean
  twofaccounts_count: number
}

export interface Preference {
  key: string
  value: unknown
  locked: boolean
}

export interface AppSettings {
  disableRegistration: boolean
  enableSso: boolean
  useSsoOnly: boolean
  enableSharing?: boolean
  enableAllUsersSharingScope?: boolean
  allowPatWhileSsoOnly?: boolean
}

export interface User {
  id: number
  name: string
  email: string
  oauth_provider: string | null
  preferences: Record<string, unknown>
  appSettings: AppSettings
  is_admin: boolean
  authenticated_by_proxy?: boolean
  email_verified?: boolean
}

export interface FrontendConfig {
  mailEnabled?: boolean
  appName: string
  appVersion: string
  subdirectory: string
  isDemoApp: boolean
  sso: { openid: boolean; github: boolean }
  appSettings: AppSettings
  features: Record<string, string>
}

export interface ManagedUser {
  id: number
  name: string
  email: string
  is_admin: boolean
  twofaccounts_count: number
  valid_personal_access_tokens: number
  webauthn_credentials: number
  created_at: string
  last_seen_at: string
}

export interface PersonalAccessToken {
  id: string
  name: string
  created_at: string | null
  revoked: boolean
}

export interface WebAuthnCredential {
  id: string
  name: string
  created_at: string
}

export interface ShareRecipient {
  id: number
  name: string
  is_shared_with: boolean
  is_shared_since?: string
}

export interface ApiError extends Error {
  status: number
  errors?: Record<string, string[]>
  needsPassphrase?: boolean
}

export interface SshKey {
  id: number
  name: string
  fingerprint: string
  key_type: string
  all_accounts: boolean
  account_ids: number[]
  group_ids: number[]
  last_used_at: number | null
  last_used_ip: string
  created_at: number
}

export interface SshKeysResponse {
  enabled: boolean
  port?: string
  /** how clients reach the service: SSH_PUBLIC_ADDR, or the host of the panel */
  host?: string
  host_fingerprint?: string
  keys: SshKey[]
}

export interface SshKeyInput {
  name: string
  all_accounts: boolean
  account_ids: string
  group_ids: string
  password: string
}
