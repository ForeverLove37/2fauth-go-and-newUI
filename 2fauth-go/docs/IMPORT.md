# Import and export

`POST /api/v1/twofaccounts/import` and the import screen of the panel read any of the
formats below; the format is detected from the content. `POST /api/v1/twofaccounts/migration`
reads the same formats without storing anything, for the upstream interface.

## Accepted formats

| Source | What to export | Encrypted files |
| --- | --- | --- |
| 2FAuth | JSON export, with its icons | yes, see below |
| Aegis | JSON export or vault | yes, password slots (scrypt) |
| 2FAS | `.2fas` backup | yes (PBKDF2) |
| Google Authenticator | the `otpauth-migration://` link of each QR code, one per line, or a picture of the QR code | no such thing |
| Bitwarden | JSON or CSV | no: export without a password |
| 1Password and other managers | CSV with an `OTPAuth`, `TOTP` or similar column | no |
| andOTP | plain JSON | no: decrypt first |
| FreeOTP+ | JSON | no |
| Authenticator Pro | JSON | no: decrypt first |
| Raivo | JSON | no |
| LastPass Authenticator | JSON | no |
| Proton Authenticator | JSON | no |
| Generic CSV | a header line naming at least a `secret` column; also `service`, `account`, `digits`, `period`, `algorithm`, `type` and `counter` | no |
| Plain text | one `otpauth://` URI, `steam://` value or `issuer:account:secret` per line | no |

Entries that cannot be used, such as an mOTP account or an invalid secret, do not stop the
others: they are reported with their reason. An account already present is skipped.

The fixtures of the tests follow the published or observed structure of each format; they are
not files produced by these applications. Check the count of an important import.

## Encrypted export

`POST /api/v1/twofaccounts/export` with `ids` and a `passphrase` of at least 12 characters
returns a JSON document. It is the regular 2FAuth export, encrypted:

```json
{
  "format": "2fauth-encrypted-v1",
  "encrypted": true,
  "app": "2fauth_v9.0.0",
  "datetime": "2026-11-03T09:41:00.000000Z",
  "kdf": { "name": "argon2id", "salt": "<base64>", "time": 3, "memory_kib": 65536, "parallelism": 4 },
  "cipher": { "name": "aes-256-gcm", "nonce": "<base64>" },
  "ciphertext": "<base64, with the 16-byte tag at the end>"
}
```

The key is 32 bytes of argon2id. The additional authenticated data of AES-GCM is the text
`2fauth-encrypted-v1`. The passphrase cannot be recovered. A file can be opened without this
application, for instance with Python:

```python
import base64, json, sys
from argon2.low_level import hash_secret_raw, Type   # pip install argon2-cffi
from cryptography.hazmat.primitives.ciphers.aead import AESGCM  # pip install cryptography

doc = json.load(open(sys.argv[1]))
kdf = doc["kdf"]
key = hash_secret_raw(sys.argv[2].encode(), base64.b64decode(kdf["salt"]), kdf["time"],
                      kdf["memory_kib"], kdf["parallelism"], 32, Type.ID)
plain = AESGCM(key).decrypt(base64.b64decode(doc["cipher"]["nonce"]),
                            base64.b64decode(doc["ciphertext"]), doc["format"].encode())
print(plain.decode())
```

Only the owner of an account can export it, in any form: a share only grants generating codes.
