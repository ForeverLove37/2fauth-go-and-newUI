# Reading codes from the command line

2FAuth has an SSH service that works like git over SSH: you register the public key of
a machine in the panel, and that machine can then read codes with a plain `ssh`
command. Nothing needs to be installed on the client.

```bash
ssh -p 2222 otp@vault.example.org get github
```

## Turning it on

The service is off by default. Set its address in `.env` and restart:

```dotenv
SSH_LISTEN_ADDR=:2222
```

The host key is created on first start in `storage/ssh_host_ed25519_key` (or
`SSH_HOST_KEY_FILE`). Keep it with your backups: if it changes, clients warn about a
changed host key. Settings → Security shows its fingerprint, so you can compare it the
first time a machine connects.

`ALLOWED_CLIENT_IPS` applies to this port as it does to HTTPS: other addresses are
disconnected before anything is read.

## Registering a machine

1. On the machine, print its public key. Create one first if there is none:

   ```bash
   ssh-keygen -t ed25519
   ```

   ```bash
   cat ~/.ssh/id_ed25519.pub
   ```

2. In Settings → Security → SSH keys, paste it, choose what it can read and confirm with
   your password.

Ed25519, ECDSA, RSA of at least 2048 bits and FIDO (`sk-`) keys are accepted. A key
belongs to one user. Key options such as `from="..."` are refused rather than ignored.

## What a key can read

A key has its own scope, either all your accounts or a chosen list of accounts and
groups. Accounts added to a chosen group later are included.

What a key reaches is that scope **and** what you can see right now: when an account
stops being shared with you, its owner deletes it or sharing is turned off, the key
loses it as well. An account outside the scope gets the same answer as one that does not
exist.

Removing a key works at once, also for a connection that is already open.

## Commands

| Command | Result |
| --- | --- |
| `get <account>` | the current code, and nothing else on standard output |
| `get <account> --json` | code, next code, seconds left, service and account |
| `list`, `list --json` | the accounts this key can read |
| `whoami` | the user, the key and how many accounts it reads |
| `help` | this list |

`<account>` is an id from `list`, a service name, `service:account`, or part of them.
`get` can be left out. When a query matches several accounts, the command lists them and
fails instead of picking one, because a wrong code is worse than no code.

The user name in the address is ignored: the key is what identifies you.

Exit status: `0` success, `1` nothing found or the key is no longer valid, `2` wrong usage
or an ambiguous query, `255` (from `ssh`) the connection or the key was refused.

```bash
CODE=$(ssh -p 2222 otp@vault.example.org get github) && echo "$CODE" | pbcopy
```

## Good to know

- Reading an **HOTP** code moves its counter, as in the panel.
- Each code read appears in the OTP log of the account, with the address of the client.
- The secret of an account never leaves the server through this service.
- There is no shell, no port forwarding and no file transfer, and passwords are not accepted.
- Connections end after a minute. An address with 20 failed sign-ins is ignored for 5 minutes.
- Keys can only be added or changed from a signed-in browser, with the password. An API
  token cannot do it, so a leaked token cannot turn into lasting access.
