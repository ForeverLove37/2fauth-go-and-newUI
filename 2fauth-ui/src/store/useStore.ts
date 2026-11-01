import { create } from 'zustand'
import { api } from '../api/client'
import type { Account, ApiError, FrontendConfig, Group, User } from '../api/types'

export interface Toast {
  id: number
  kind: 'info' | 'success' | 'error' | 'warn'
  text: string
}

type Prefs = Record<string, unknown>

interface State {
  booting: boolean
  ready: boolean
  config: FrontendConfig | null
  user: User | null
  accounts: Account[]
  groups: Group[]
  prefs: Prefs
  locked: string[]
  selectedId: number | null
  toasts: Toast[]
  busy: boolean

  boot: () => Promise<void>
  login: (email: string, password: string) => Promise<void>
  register: (name: string, email: string, password: string, confirmation: string) => Promise<void>
  logout: () => Promise<void>
  refreshSession: () => Promise<void>
  loadAccounts: (withOtp?: boolean) => Promise<void>
  loadGroups: () => Promise<void>
  select: (id: number | null) => void
  setPreference: (key: string, value: unknown) => Promise<void>
  toggleFavorite: (id: number) => Promise<void>
  toast: (kind: Toast['kind'], text: string) => void
  dismiss: (id: number) => void
}

let toastSeq = 0

function lockedFrom(list: { key: string; locked: boolean }[]): string[] {
  return list.filter((item) => item.locked).map((item) => item.key)
}

export const useStore = create<State>((set, get) => ({
  booting: true,
  ready: false,
  config: null,
  user: null,
  accounts: [],
  groups: [],
  prefs: {},
  locked: [],
  selectedId: null,
  toasts: [],
  busy: false,

  async boot() {
    try {
      const config = await api.config()
      set({ config })
    } catch {
      // the panel still works without the public config endpoint
    }
    try {
      await get().refreshSession()
    } catch {
      set({ user: null })
    } finally {
      set({ booting: false, ready: true })
    }
  },

  async refreshSession() {
    const user = await api.me()
    set({ user })
    const [preferences, groups] = await Promise.all([api.preferences(), api.groups()])
    const prefs: Prefs = {}
    for (const preference of preferences) prefs[preference.key] = preference.value
    set({ prefs, locked: lockedFrom(preferences), groups, accounts: [], selectedId: null })
    await get().loadAccounts(true)
  },

  async login(email, password) {
    const payload = await api.login(email, password)
    set({ user: payload as unknown as User })
    await get().refreshSession()
  },

  async register(name, email, password, confirmation) {
    const payload = await api.register(name, email, password, confirmation)
    set({ user: payload as unknown as User })
    await get().refreshSession()
  },

  async logout() {
    try {
      await api.logout()
    } catch {
      /* the session may already be gone */
    }
    set({ user: null, accounts: [], groups: [], prefs: {}, selectedId: null })
  },

  async loadAccounts(withOtp = true) {
    const accounts = await api.accounts(withOtp)
    const current = get().selectedId
    const stillThere = current !== null && accounts.some((account) => account.id === current)
    set({ accounts, selectedId: stillThere ? current : (accounts[0]?.id ?? null) })
  },

  async loadGroups() {
    set({ groups: await api.groups() })
  },

  select(id) {
    set({ selectedId: id })
  },

  async setPreference(key, value) {
    const previous = get().prefs[key]
    set({ prefs: { ...get().prefs, [key]: value } })
    try {
      await api.setPreference(key, value)
    } catch (error) {
      set({ prefs: { ...get().prefs, [key]: previous } })
      const typed = error as ApiError
      get().toast('error', typed.message)
    }
  },

  async toggleFavorite(id) {
    const account = get().accounts.find((item) => item.id === id)
    if (!account) return
    const next = !account.is_favorite
    set({
      accounts: get().accounts.map((item) => (item.id === id ? { ...item, is_favorite: next } : item)),
    })
    try {
      await api.favorite(id, next)
      get().toast('success', next ? `${account.service || account.account} pinned` : 'Unpinned')
    } catch (error) {
      set({
        accounts: get().accounts.map((item) =>
          item.id === id ? { ...item, is_favorite: !next } : item,
        ),
      })
      get().toast('error', (error as ApiError).message)
    }
  },

  toast(kind, text) {
    const id = ++toastSeq
    set({ toasts: [...get().toasts, { id, kind, text }] })
    window.setTimeout(() => get().dismiss(id), kind === 'error' ? 6000 : 3200)
  },

  dismiss(id) {
    set({ toasts: get().toasts.filter((toast) => toast.id !== id) })
  },
}))

// helper used by views to surface an error consistently
export function report(error: unknown, fallback = 'Something went wrong') {
  const typed = error as ApiError
  const message = typed?.message || fallback
  useStore.getState().toast('error', message)
  return message
}
