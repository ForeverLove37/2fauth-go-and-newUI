# Transactional e-mails

Templates live in `templates/`: `<kind>.html.tmpl` (html/template, inside
`layout.html.tmpl`) and `<kind>.txt.tmpl` (text/template, blocks `subject`,
`preheader` and `text`). Each kind is written in zh-CN and en; `mail.Lang` picks the
language from the `lang` preference of the recipient, then `MAIL_LOCALE`.

Render every template with sample values:

```bash
2fauth mail:preview ./previews https://2fauth.example.org
```

## Variables

Every variable is a required plain-text string. The engines escape it for its context;
never pass HTML. Names ending in `URL` must be absolute http(s) links, otherwise
`Render` fails. Dates are formatted by the caller, in the time zone of the recipient.
`Account` may be empty, in which case its line is left out.

| Template | Variables |
| --- | --- |
| `reset_password` | `Email`, `ResetURL`, `ExpiresMinutes` |
| `verify_email` | `Email`, `VerifyURL`, `ExpiresHours` |
| `new_device` | `Email`, `Time`, `IP`, `Browser`, `Platform`, `Method`, `SecurityURL` |
| `failed_login` | `Email`, `Time`, `IP`, `Browser`, `Platform`, `SecurityURL` |
| `password_changed` | `Email`, `Time`, `IP`, `ResetURL` |
| `account_shared` | `OwnerName`, `Label`, `Account`, `AppURL` |
| `account_transferred` | `OwnerName`, `Label`, `Account`, `AppURL` |
| `test_email` | `Server`, `Encryption`, `Time`, `AdminName` |

`Method` is `password` or anything else for single sign-on; the template translates it.
Long names and addresses wrap (`word-break`). Secrets and codes are never put in an
e-mail. Key content stays HTML text, and the layout uses no image, so nothing is lost
when a client blocks remote content.
