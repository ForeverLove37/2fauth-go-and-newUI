package httpapi

import "net/http"

// registerRoutes builds the routing table. Order matters: the first matching
// route with the highest literal-score wins, mirroring Laravel's behaviour.
func (s *Server) registerRoutes() {
	// --- public / anonymous -------------------------------------------------
	s.add(http.MethodPost, "user", s.register)
	s.add(http.MethodPost, "user/login", s.login)
	s.add(http.MethodGet, "user/logout", s.logout)
	s.add(http.MethodPost, "user/logout", s.logout)
	s.add(http.MethodGet, "user/email/verify", s.verifyEmail)
	s.add(http.MethodPost, "user/password/lost", s.passwordLost)
	s.add(http.MethodPost, "user/password/reset", s.passwordReset)
	s.add(http.MethodGet, "refresh-csrf", s.refreshCSRF)
	s.add(http.MethodGet, "manifest.json", s.manifestJSON)
	s.add(http.MethodGet, "up", s.healthPage)

	// --- socialite ----------------------------------------------------------
	// Only the OpenID Connect driver is implemented; any other driver answers
	// 501 like the pre-SSO build did.
	s.add(http.MethodGet, "socialite/redirect/{driver}", s.socialiteRedirect)
	s.add(http.MethodGet, "socialite/callback/{driver}", s.socialiteCallback)

	// --- single sign-on (OpenID Connect) ------------------------------------
	s.add(http.MethodGet, "api/v1/sso/config", s.ssoConfig)
	s.addAdmin(http.MethodGet, "api/v1/sso/settings", s.showSSOSettings)
	s.addAdmin(http.MethodPut, "api/v1/sso/settings", s.updateSSOSettings)
	s.addAdmin(http.MethodPost, "api/v1/sso/test", s.testSSO)
	s.addAdmin(http.MethodGet, "api/v1/sso/presets", s.showSSOPresets)
	s.addAdmin(http.MethodGet, "api/v1/sso/keycloak-client", s.keycloakClient)

	// --- authenticated user -------------------------------------------------
	s.addAuth(http.MethodPut, "user", s.updateUser)
	s.addAuth(http.MethodDelete, "user", s.deleteUser)
	s.addAuth(http.MethodPatch, "user/password", s.updatePassword)
	s.addAuth(http.MethodPost, "user/email/verification", s.sendEmailVerification)

	// personal access tokens
	s.addAuth(http.MethodGet, "oauth/personal-access-tokens", s.listPersonalAccessTokens)
	s.addAuth(http.MethodPost, "oauth/personal-access-tokens", s.createPersonalAccessToken)
	s.addAuth(http.MethodDelete, "oauth/personal-access-tokens/{token_id}", s.deletePersonalAccessToken)

	// SSH keys of the command line service
	s.addAuth(http.MethodGet, "api/v1/ssh-keys", s.listSSHKeys)
	s.addAuth(http.MethodPost, "api/v1/ssh-keys", s.createSSHKey)
	s.addAuth(http.MethodPut, "api/v1/ssh-keys/{key}", s.updateSSHKey)
	s.addAuth(http.MethodDelete, "api/v1/ssh-keys/{key}", s.deleteSSHKey)

	// webauthn
	s.add(http.MethodPost, "webauthn/login/options", s.webauthnLoginOptions)
	s.add(http.MethodPost, "webauthn/login", s.webauthnLogin)
	s.add(http.MethodPost, "webauthn/lost", s.webauthnLost)
	s.add(http.MethodPost, "webauthn/recover", s.webauthnRecover)
	s.addAuth(http.MethodPost, "webauthn/register/options", s.webauthnRegisterOptions)
	s.addAuth(http.MethodPost, "webauthn/register", s.webauthnRegister)
	s.addAuth(http.MethodGet, "webauthn/credentials", s.webauthnCredentials)
	s.addAuth(http.MethodPatch, "webauthn/credentials/{credential}/name", s.webauthnRenameCredential)
	s.addAuth(http.MethodDelete, "webauthn/credentials/{credential}", s.webauthnDeleteCredential)

	// administration
	s.addAdmin(http.MethodGet, "system/infos", s.systemInfos)
	s.addAdmin(http.MethodPost, "system/test-email", s.systemTestEmail)
	s.addAdmin(http.MethodGet, "api/v1/mail/settings", s.showMailSettings)
	s.addAdmin(http.MethodPut, "api/v1/mail/settings", s.updateMailSettings)
	s.addAdmin(http.MethodGet, "system/latestRelease", s.systemLatestRelease)
	s.addAdmin(http.MethodGet, "system/optimize", s.systemOptimize)
	s.addAdmin(http.MethodGet, "system/clear-cache", s.systemClearCache)
	s.addAdmin(http.MethodGet, "api/v1/users", s.listUsers)
	s.addAdmin(http.MethodPost, "api/v1/users", s.storeUser)
	s.addAdmin(http.MethodGet, "api/v1/users/{user}", s.showUser)
	s.addAdmin(http.MethodDelete, "api/v1/users/{user}", s.destroyUser)
	s.addAdmin(http.MethodPatch, "api/v1/users/{user}/password/reset", s.resetUserPassword)
	s.addAdmin(http.MethodPatch, "api/v1/users/{user}/promote", s.promoteUser)
	s.addAdmin(http.MethodDelete, "api/v1/users/{user}/pats", s.revokeUserPATs)
	s.addAdmin(http.MethodDelete, "api/v1/users/{user}/credentials", s.revokeUserCredentials)
	s.addAdmin(http.MethodGet, "api/v1/users/{user}/authentications", s.userAuthentications)

	s.addAdmin(http.MethodGet, "api/v1/settings", s.listSettings)
	s.addAdmin(http.MethodPost, "api/v1/settings", s.storeSetting)
	s.addAdmin(http.MethodGet, "api/v1/settings/{settingName}", s.showSetting)
	s.addAdmin(http.MethodPut, "api/v1/settings/{settingName}", s.updateSetting)
	s.addAdmin(http.MethodDelete, "api/v1/settings/{settingName}", s.deleteSetting)

	// --- API v1 -------------------------------------------------------------
	s.add(http.MethodGet, "api/v1/user/name", s.deprecatedUserName)
	s.addAuth(http.MethodGet, "api/v1/user", s.apiUser)
	s.addAuth(http.MethodGet, "api/v1/user/preferences", s.allPreferences)
	s.addAuth(http.MethodGet, "api/v1/user/preferences/{preferenceName}", s.showPreference)
	s.addAuth(http.MethodPut, "api/v1/user/preferences/{preferenceName}", s.setPreference)

	// twofaccounts: static sub-paths first (the router prefers literal matches,
	// this ordering only documents intent).
	s.addAuth(http.MethodDelete, "api/v1/twofaccounts", s.batchDestroyAccounts)
	s.addAuth(http.MethodPatch, "api/v1/twofaccounts/withdraw", s.withdrawAccounts)
	s.addAuth(http.MethodPost, "api/v1/twofaccounts/reorder", s.reorderAccounts)
	s.addAuth(http.MethodPost, "api/v1/twofaccounts/migration", s.migrateAccounts)
	s.limitBody(importMaxBody)
	s.addAuth(http.MethodPost, "api/v1/twofaccounts/import", s.importAccounts)
	s.limitBody(importMaxBody)
	s.addAuth(http.MethodPost, "api/v1/twofaccounts/export", s.exportEncrypted)
	s.addAuth(http.MethodPost, "api/v1/twofaccounts/preview", s.previewAccount)
	s.addAuth(http.MethodGet, "api/v1/twofaccounts/export", s.exportAccounts)
	s.addAuth(http.MethodGet, "api/v1/twofaccounts/count", s.countAccounts)
	s.addAuth(http.MethodPost, "api/v1/twofaccounts/otp", s.otpFromParameters)
	s.addAuth(http.MethodPost, "api/v1/twofaccounts/icons/fill", s.fillMissingIcons)
	s.addAuth(http.MethodGet, "api/v1/twofaccounts", s.listAccounts)
	s.addAuth(http.MethodPost, "api/v1/twofaccounts", s.storeAccount)
	s.addAuth(http.MethodGet, "api/v1/twofaccounts/{twofaccount}/qrcode", s.accountQrCode)
	s.addAuth(http.MethodGet, "api/v1/twofaccounts/{twofaccount}/otp-logs", s.accountOtpLogs)
	s.addAuth(http.MethodPatch, "api/v1/twofaccounts/{twofaccount}/favorite", s.favoriteAccount)
	s.addAuth(http.MethodPatch, "api/v1/twofaccounts/{twofaccount}/owner", s.transferOwnership)
	s.addAuth(http.MethodGet, "api/v1/twofaccounts/{twofaccount}/shares", s.listShares)
	s.addAuth(http.MethodPost, "api/v1/twofaccounts/{twofaccount}/shares", s.createShares)
	s.addAuth(http.MethodDelete, "api/v1/twofaccounts/{twofaccount}/shares", s.deleteAllShares)
	s.addAuth(http.MethodPost, "api/v1/twofaccounts/{twofaccount}/shares/all", s.shareWithAll)
	s.addAuth(http.MethodDelete, "api/v1/twofaccounts/{twofaccount}/shares/{user}", s.deleteShare)
	s.addAuth(http.MethodGet, "api/v1/twofaccounts/{twofaccount}/recipients", s.shareRecipients)
	s.addAuth(http.MethodGet, "api/v1/twofaccounts/{id}/otp", s.otpByID)
	s.addAuth(http.MethodGet, "api/v1/twofaccounts/{twofaccount}", s.showAccount)
	s.addAuth(http.MethodPut, "api/v1/twofaccounts/{twofaccount}", s.updateAccount)
	s.addAuth(http.MethodPatch, "api/v1/twofaccounts/{twofaccount}", s.updateAccount)
	s.addAuth(http.MethodDelete, "api/v1/twofaccounts/{twofaccount}", s.destroyAccount)

	// groups
	s.addAuth(http.MethodGet, "api/v1/groups/{group}/twofaccounts", s.groupAccounts)
	s.addAuth(http.MethodPost, "api/v1/groups/{group}/assign", s.assignAccountsToGroup)
	s.addAuth(http.MethodPost, "api/v1/groups/reorder", s.reorderGroups)
	s.addAuth(http.MethodGet, "api/v1/groups", s.listGroups)
	s.addAuth(http.MethodPost, "api/v1/groups", s.storeGroup)
	s.addAuth(http.MethodGet, "api/v1/groups/{group}", s.showGroup)
	s.addAuth(http.MethodPut, "api/v1/groups/{group}", s.updateGroup)
	s.addAuth(http.MethodPatch, "api/v1/groups/{group}", s.updateGroup)
	s.addAuth(http.MethodDelete, "api/v1/groups/{group}", s.destroyGroup)

	// qrcode
	s.addAuth(http.MethodPost, "api/v1/qrcode/decode", s.decodeQrCode)
	s.limitBody(uploadMaxBody)

	// icons
	s.addAuth(http.MethodGet, "api/v1/icons/packs", s.iconPacks)
	s.addAuth(http.MethodGet, "api/v1/icons/providers", s.iconProviders)
	s.addAdmin(http.MethodPut, "api/v1/icons/providers", s.updateIconProviders)
	s.addAuth(http.MethodPost, "api/v1/icons/default", s.iconFetch)
	s.addAuth(http.MethodPost, "api/v1/icons", s.iconUpload)
	s.limitBody(uploadMaxBody)
	s.addAuth(http.MethodDelete, "api/v1/icons/{icon}", s.iconDelete)

	// feature flags
	s.addAuth(http.MethodGet, "api/v1/features", s.featuresIndex)
	s.addAuth(http.MethodGet, "api/v1/features/{feature}", s.featureShow)

	// React panel
	s.add(http.MethodGet, "api/v1/frontend-config", s.frontendConfig)
	s.addStatic(http.MethodGet, "assets/{file...}", s.serveUIAsset)
	s.addStatic(http.MethodGet, "favicon.svg", s.serveUIFavicon)

	// static assets (lowest priority: registered last)
	s.addStatic(http.MethodGet, "build/{file...}", s.serveBuildAsset)
	s.addStatic(http.MethodGet, "storage/icons/{file...}", s.serveIconRoute)
	s.addStatic(http.MethodGet, "favicon.ico", s.servePublicFile)
	s.addStatic(http.MethodGet, "favicon.png", s.servePublicFile)
	s.addStatic(http.MethodGet, "favicon_lg.png", s.servePublicFile)
	s.addStatic(http.MethodGet, "logo.svg", s.servePublicFile)
	s.addStatic(http.MethodGet, "robots.txt", s.servePublicFile)
	s.addStatic(http.MethodGet, "healthcheck.css", s.servePublicFile)
}
