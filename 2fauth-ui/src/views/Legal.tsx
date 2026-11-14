import { Link } from 'react-router-dom'
import Logo from '../components/Logo'
import { resolveLocale, useI18nStore, useT } from '../i18n'
import { useStore } from '../store/useStore'
import { LegalFooter, REPOSITORY } from './Landing'

export type LegalKind = 'terms' | 'privacy'

const UPDATED = '2026-11-14'

type Section = { title: string; body: (string | string[])[] }
type Doc = { title: string; intro: string; sections: Section[] }

/**
 * The terms and the privacy policy of a server running this software. They are
 * written for the operator of the server: whoever installed it and runs it.
 */
function documents(app: string): Record<'en' | 'zh', Record<LegalKind, Doc>> {
  return {
    en: {
      terms: {
        title: 'Terms of use',
        intro: `These terms apply to the use of ${app} at this address: the web panel, its API, the SSH access for agents and the Android app connected to it. ${app} is a self-hosted instance of open-source software, run by the person or organisation that installed it on this server (“the operator”). By creating an account or using the service you accept these terms.`,
        sections: [
          {
            title: '1. The service',
            body: [
              `${app} stores the secrets of your two-factor authentication (TOTP, HOTP and Steam) and turns them into one-time codes. It is a tool to help you sign in to other services; it is not those services, and their own terms still apply to your accounts there.`,
              'The operator may decide who can register, may limit access to some networks, and may enable or disable features such as single sign-on, sharing or SSH access.',
            ],
          },
          {
            title: '2. Your account',
            body: [
              [
                'Keep your password, passkeys and recovery means to yourself; you are responsible for what is done with your account.',
                'Give an email address you can read: it is used to reset your password and for messages about your account.',
                'Tokens, device tokens and SSH keys act on your behalf. Only grant them to machines and agents you trust, give them the smallest scope that works, and revoke those you no longer use.',
              ],
            ],
          },
          {
            title: '3. Keep a way back in',
            body: [
              'Losing access to a two-factor code can lock you out of another service. Keep the recovery codes those services give you, export an encrypted backup of your accounts from time to time, and do not rely on a single copy of a secret.',
              'Codes kept only on your phone are not on this server: uninstalling the app, resetting the phone or losing it deletes them, and the operator cannot recover them.',
            ],
          },
          {
            title: '4. Acceptable use',
            body: [
              [
                'Only store secrets of accounts you are entitled to use.',
                'Do not try to reach other users’ data, guess passwords or keys, get around rate limits or network restrictions, or disturb the service.',
                'Do not use the service for anything unlawful.',
              ],
              'Report a security problem to the operator, or for the software itself through the repository, before making it public.',
            ],
          },
          {
            title: '5. Your data',
            body: [
              'What you store remains yours. The operator processes it only to run the service, as described in the privacy policy. You can export your accounts, and delete your account at any time from the settings (the only administrator of a server cannot delete their own account).',
            ],
          },
          {
            title: '6. Sharing',
            body: [
              'When you share an account with another user of this server, that user can obtain its codes until you stop sharing it. Every code handed out is recorded in the log of the account, which you can read.',
            ],
          },
          {
            title: '7. The Android app',
            body: [
              'The app is distributed from this server and from the project’s repository, outside of app stores. Install it only from those sources; each update must be signed with the same key as the version you have, or Android refuses it.',
            ],
          },
          {
            title: '8. Availability and changes',
            body: [
              'The service is run on a best-effort basis. The operator may change it, interrupt it for maintenance, or stop it. When the operator plans to stop the service, they will try to give notice so you can export your data.',
              'The operator may suspend an account that breaks these terms or endangers the service or other users.',
            ],
          },
          {
            title: '9. No warranty',
            body: [
              `The service and the software are provided “as is”, without warranty of any kind. As far as the law allows, the operator is not liable for indirect damage, for the loss of data, or for being locked out of another service. Nothing in these terms limits a liability that cannot be limited by law.`,
            ],
          },
          {
            title: '10. The software',
            body: [
              `The source code is published under the GNU Affero General Public License v3.0 at ${REPOSITORY}. These terms cover the use of this server, not the code: the license covers the code.`,
            ],
          },
          {
            title: '11. Changes to these terms',
            body: ['The operator may update these terms. The date at the top of this page tells which version applies; using the service after a change means you accept the new version.'],
          },
        ],
      },
      privacy: {
        title: 'Privacy policy',
        intro: `This policy explains what ${app} at this address stores about you, why, and for how long. The operator of this server is responsible for it. There is no advertising, no analytics, no tracking, and nothing is sold.`,
        sections: [
          {
            title: '1. What is stored',
            body: [
              [
                'Your account: name, email address, a bcrypt hash of your password (never the password itself), the public part of your passkeys, the identity your single sign-on provider gives (if you use one), and your preferences.',
                'Your two-factor accounts: service, account name, icon, group, and the secret with its parameters. Secrets are encrypted at rest with the key of the server. The server must read them to compute codes, so this is not end-to-end encryption: whoever controls the server can read them.',
                'Access you grant: tokens, device tokens of the Android app, and the public keys of the SSH keys of your agents, with their scope and when they were last used.',
                'Sign-in log: date, IP address, browser or app, and whether the attempt succeeded; kept 365 days by default.',
                'Code log: each code handed out (in the panel, the app, the API or over SSH), with who asked, the IP address and the date; kept 365 days by default.',
              ],
            ],
          },
          {
            title: '2. Codes kept on your phone',
            body: [
              'In the Android app, codes saved “on this phone” never reach this server: the secret is stored on the phone, encrypted with a key held by Android. Only synced codes have a copy here, and only after you have confirmed with your fingerprint, face or screen lock. Cloud codes stay on the server; the phone only receives short-lived codes for them.',
            ],
          },
          {
            title: '3. Cookies and local storage',
            body: [
              'Two cookies are set when you sign in, and both are needed for the service to work: an encrypted session cookie and a CSRF protection cookie. Your browser also keeps your theme, language and layout in its local storage. There are no third-party cookies.',
            ],
          },
          {
            title: '4. Who else is involved',
            body: [
              [
                'Icons: when you ask the server for the icon of a service, it fetches it from public icon libraries (for example selfh.st, dashboard-icons or Simple Icons through jsDelivr). They see the name of the service and the address of this server, not who you are.',
                'Email: password resets and notifications go through the mail provider the operator configured.',
                'Single sign-on: if you sign in with an external provider, that provider knows you signed in here.',
                'Network: this server may be reached through a content delivery network or a tunnel (such as Cloudflare), which processes the traffic to deliver it.',
                'Android app: for codes kept on the phone, the app can fetch icons from public icon libraries, which then see the name of the service; you can turn this off in its settings. It checks this server for updates twice a day.',
              ],
              'This page and the home page load nothing from third parties.',
            ],
          },
          {
            title: '5. Why',
            body: [
              'Everything listed is used only to provide the service you asked for: to sign you in, compute your codes, keep your devices in sync, protect your account (the logs help you and the operator spot misuse), and send you the emails you need. Nothing is used for profiling or marketing.',
            ],
          },
          {
            title: '6. How long',
            body: [
              'Your data stays until you delete it or your account. Deleting your account removes your two-factor accounts, groups, tokens, SSH keys, passkeys, sessions, preferences and the logs about them from the database. Otherwise, logs expire after their retention period. Backups made by the operator may keep a copy until they are themselves replaced.',
            ],
          },
          {
            title: '7. Your rights',
            body: [
              'You can see and correct your data in the panel, export your accounts, and delete your account from the settings. For anything else, including a copy of the logs about you or a question about this policy, contact the operator of this server. Depending on where you live, you may also have the right to complain to a data protection authority.',
            ],
          },
          {
            title: '8. Security',
            body: [
              'Connections are encrypted, secrets are encrypted at rest, sessions are protected against cross-site requests, sign-in attempts are rate-limited, and the operator can restrict access to some networks. No system is perfectly secure: keep your recovery codes.',
            ],
          },
          {
            title: '9. Changes',
            body: ['The operator may update this policy. The date at the top of this page tells which version applies.'],
          },
        ],
      },
    },
    zh: {
      terms: {
        title: '使用条款',
        intro: `本条款适用于你在本地址使用 ${app}，包括网页面板、API、供 Agent 使用的 SSH 访问，以及连接到本服务器的 Android 应用。${app} 是开源软件的自托管实例，由在本服务器上安装并运行它的个人或组织（“运营者”）负责。注册账户或使用本服务，即表示你接受本条款。`,
        sections: [
          {
            title: '1. 服务内容',
            body: [
              `${app} 保存你的两步验证密钥（TOTP、HOTP、Steam），并据此生成一次性验证码。它是帮助你登录其他服务的工具，并不代表那些服务；你在那些服务上的账户仍受其各自条款约束。`,
              '运营者可以决定谁能注册、限制只允许部分网络访问，并开启或关闭单点登录、共享、SSH 访问等功能。',
            ],
          },
          {
            title: '2. 你的账户',
            body: [
              [
                '请妥善保管你的密码、通行密钥和恢复方式；账户下发生的操作由你负责。',
                '请填写你能收到邮件的邮箱：它用于重置密码和发送与账户相关的通知。',
                '令牌、设备令牌和 SSH 密钥代表你执行操作。只授予你信任的机器和 Agent，范围尽量小，不再使用的及时吊销。',
              ],
            ],
          },
          {
            title: '3. 给自己留一条后路',
            body: [
              '丢失两步验证码可能导致你无法登录其他服务。请保存各服务提供的恢复码，定期导出账户的加密备份，不要让某个密钥只有一份副本。',
              '只保存在手机上的验证码不在本服务器上：卸载应用、重置或丢失手机都会让它们消失，运营者无法恢复。',
            ],
          },
          {
            title: '4. 合理使用',
            body: [
              ['只保存你有权使用的账户的密钥。', '不得尝试获取其他用户的数据、猜测密码或密钥、绕过频率限制或网络限制，或干扰服务运行。', '不得将本服务用于任何违法用途。'],
              '发现安全问题，请先报告给运营者（软件本身的问题可通过代码仓库报告），再公开。',
            ],
          },
          {
            title: '5. 你的数据',
            body: ['你保存的内容归你所有。运营者只为运行服务而处理这些数据，具体见隐私政策。你可以导出账户，也可以随时在设置中删除自己的账户（服务器唯一的管理员不能删除自己的账户）。'],
          },
          {
            title: '6. 共享',
            body: ['你把账户共享给本服务器的其他用户后，对方在你取消共享之前都可以获取它的验证码。每一次发放验证码都会记录在该账户的日志中，你可以查看。'],
          },
          {
            title: '7. Android 应用',
            body: ['应用通过本服务器和项目代码仓库分发，不经过应用商店。请只从这两个来源安装；每次更新都必须使用与已安装版本相同的密钥签名，否则 Android 会拒绝安装。'],
          },
          {
            title: '8. 可用性与变更',
            body: [
              '本服务尽力提供。运营者可能调整服务、因维护暂停服务或停止服务。计划停止服务时，运营者会尽量提前通知，以便你导出数据。',
              '对于违反本条款、危害服务或其他用户的账户，运营者可以暂停其使用。',
            ],
          },
          {
            title: '9. 免责声明',
            body: ['本服务和软件按“现状”提供，不作任何形式的保证。在法律允许的范围内，运营者不对间接损失、数据丢失或因此无法登录其他服务承担责任。本条款不限制法律规定不得限制的责任。'],
          },
          {
            title: '10. 软件许可',
            body: [`源代码以 GNU Affero General Public License v3.0 发布于 ${REPOSITORY}。本条款约束的是本服务器的使用，代码本身受该许可证约束。`],
          },
          {
            title: '11. 条款变更',
            body: ['运营者可能更新本条款。以页面顶部的日期为准；变更后继续使用本服务，即表示你接受新版本。'],
          },
        ],
      },
      privacy: {
        title: '隐私政策',
        intro: `本政策说明本地址上的 ${app} 会保存你的哪些信息、为什么保存、保存多久。由本服务器的运营者负责。这里没有广告、没有统计分析、没有追踪，也不出售任何数据。`,
        sections: [
          {
            title: '1. 保存哪些信息',
            body: [
              [
                '你的账户：名称、邮箱、密码的 bcrypt 哈希（不保存密码本身）、通行密钥的公钥部分、单点登录提供方给出的身份（如果你使用），以及你的偏好设置。',
                '你的两步验证账户：服务名、账户名、图标、分组，以及密钥和参数。密钥使用服务器的密钥加密存储。服务器需要读取密钥才能计算验证码，所以这不是端到端加密：能控制服务器的人可以读取它们。',
                '你授予的访问：令牌、Android 应用的设备令牌、Agent 使用的 SSH 公钥，以及它们的范围和最后使用时间。',
                '登录日志：时间、IP 地址、浏览器或应用、是否成功；默认保留 365 天。',
                '验证码日志：每一次发放验证码（面板、应用、API 或 SSH），包括请求者、IP 地址和时间；默认保留 365 天。',
              ],
            ],
          },
          {
            title: '2. 保存在手机上的验证码',
            body: ['在 Android 应用中，保存为“仅本机”的验证码永远不会发送到本服务器：密钥保存在手机上，并用 Android 系统持有的密钥加密。只有同步的验证码在这里有副本，而且必须先通过指纹、面容或锁屏密码确认。云端验证码保存在服务器上，手机只会收到短效验证码。'],
          },
          {
            title: '3. Cookie 与本地存储',
            body: ['登录时会设置两个 Cookie，都是服务运行所必需的：一个加密的会话 Cookie 和一个防 CSRF 的 Cookie。你的浏览器还会在本地存储中保存主题、语言和布局。没有任何第三方 Cookie。'],
          },
          {
            title: '4. 涉及的第三方',
            body: [
              [
                '图标：当你让服务器获取某个服务的图标时，服务器会从公共图标库（例如通过 jsDelivr 访问的 selfh.st、dashboard-icons 或 Simple Icons）下载。它们能看到服务名和本服务器的地址，看不到你是谁。',
                '邮件：重置密码和通知邮件通过运营者配置的邮件服务商发送。',
                '单点登录：如果你使用外部身份提供方登录，该提供方会知道你在这里登录过。',
                '网络：本服务器可能通过内容分发网络或隧道（例如 Cloudflare）访问，它们为传输流量而处理这些流量。',
                'Android 应用：对于仅本机的验证码，应用可以从公共图标库获取图标，图标库会看到服务名；可以在应用设置中关闭。应用每天会向本服务器检查两次更新。',
              ],
              '本页面和首页不会从第三方加载任何内容。',
            ],
          },
          {
            title: '5. 用途',
            body: ['以上信息只用于提供你所请求的服务：让你登录、计算验证码、在设备之间同步、保护你的账户（日志帮助你和运营者发现滥用），以及发送你需要的邮件。不会用于用户画像或营销。'],
          },
          {
            title: '6. 保存期限',
            body: ['你的数据会一直保存，直到你删除它或删除账户。删除账户会从数据库中移除你的两步验证账户、分组、令牌、SSH 密钥、通行密钥、会话、偏好设置以及与之相关的日志。除此之外，日志在保留期结束后自动删除。运营者做的备份可能在被替换之前仍保留一份副本。'],
          },
          {
            title: '7. 你的权利',
            body: ['你可以在面板中查看和修改你的数据、导出账户，并在设置中删除账户。其他请求，包括获取与你有关的日志副本或对本政策的疑问，请联系本服务器的运营者。根据你所在地的法律，你可能还有权向数据保护机构投诉。'],
          },
          {
            title: '8. 安全',
            body: ['连接经过加密，密钥加密存储，会话防跨站请求，登录尝试有频率限制，运营者还可以限制只允许部分网络访问。没有绝对安全的系统：请保存好你的恢复码。'],
          },
          {
            title: '9. 政策变更',
            body: ['运营者可能更新本政策，以页面顶部的日期为准。'],
          },
        ],
      },
    },
  }
}

function Paragraphs({ body }: { body: Section['body'] }) {
  return (
    <>
      {body.map((part, index) =>
        Array.isArray(part) ? (
          <ul key={index}>
            {part.map((item) => (
              <li key={item}>{item}</li>
            ))}
          </ul>
        ) : (
          <p key={index}>{part}</p>
        ),
      )}
    </>
  )
}

/** The terms of use or the privacy policy, in Chinese or English. */
export default function Legal({ kind }: { kind: LegalKind }) {
  const t = useT()
  const config = useStore((state) => state.config)
  const user = useStore((state) => state.user)
  const locale = resolveLocale(useI18nStore((state) => state.locale))
  const appName = config?.appName ?? '2FAuth'
  const doc = documents(appName)[locale === 'en' ? 'en' : 'zh'][kind]

  return (
    <div className="doc-page">
      <header className="doc-top">
        <Link to="/" className="brand">
          <Logo />
          <div>
            {appName}
            <small>{doc.title}</small>
          </div>
        </Link>
        <div className="spacer" />
        <Link className="btn small ghost" to={user ? '/' : '/login'}>
          {user ? t('Back to the panel') : t('Sign in')}
        </Link>
      </header>

      <main className="doc">
        <article className="panel doc-body legal">
          <div className="legal-switch">
            <Link className={`btn small${kind === 'terms' ? ' primary' : ' ghost'}`} to="/terms">
              {t('Terms')}
            </Link>
            <Link className={`btn small${kind === 'privacy' ? ' primary' : ' ghost'}`} to="/privacy">
              {t('Privacy')}
            </Link>
          </div>
          <h1>{doc.title}</h1>
          <p className="legal-updated">{t('Last updated {date}', { date: UPDATED })}</p>
          <p className="legal-note">{doc.intro}</p>
          {doc.sections.map((section) => (
            <section key={section.title}>
              <h2>{section.title}</h2>
              <Paragraphs body={section.body} />
            </section>
          ))}
        </article>
      </main>
      <LegalFooter appName={appName} />
    </div>
  )
}
