/** The tabs of the settings screen, split between the two entries of the top bar. */
export const personalTabs = ['theme', 'options', 'account', 'security'] as const
export const adminTabs = ['admin', 'sso', 'mail', 'android'] as const

export type SettingsTab = (typeof personalTabs)[number] | (typeof adminTabs)[number]

export function isAdminTab(tab: string | null): boolean {
  return (adminTabs as readonly string[]).includes(tab ?? '')
}
