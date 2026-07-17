# Cookiex MVP Design

## Product goal

Cookiex is a Linux-first CLI that securely bridges an authenticated local
Chrome/Chromium session into terminal HTTP workflows. It reads cookies already
stored by the selected browser profile; it does not obtain sessions for sites
the user has not logged into.

The first release focuses on one repeatable workflow:

```text
select Chrome profile → import cookies for one host → save encrypted snapshot
→ send a request or export runnable code → explicitly sync when needed
```

Cookiex cannot guarantee that every authenticated browser session is
replayable. Some sites additionally depend on CSRF tokens, local storage,
browser-only headers, TLS fingerprints, IP binding, or device attestation.

## MVP commands

```bash
cookiex import github.com --profile work
cookiex send GET https://github.com/settings --profile work
cookiex export work --domain github.com --format curl
cookiex sync work
cookiex profiles
```

- Import is domain-scoped by default. There is no implicit bulk export.
- Multiple Chrome profiles trigger a first-run selection that is remembered.
- Cookie profiles are snapshots. Chrome is read again only on explicit sync.
- Values are redacted in normal output.
- Export initially supports curl, Python requests, and JavaScript fetch.

## Architecture

```text
CLI
 ├── Chrome Reader
 ├── Cookie Matcher
 ├── Encrypted Vault
 ├── HTTP Runner
 └── Code Exporters
```

The Chrome Reader discovers Linux Chrome and Chromium data directories, copies
the selected `Cookies` SQLite database before reading it, and decrypts supported
Linux OSCrypt values. The Cookie Matcher applies host-only/domain, path,
secure, and expiry semantics. The Encrypted Vault stores authenticated
ciphertext on disk and keeps its random master key in Linux Secret Service.
The HTTP Runner and exporters share the same matcher.

## Safety boundaries

- Never write to Chrome's cookie database.
- Never merge cookies from different browser profiles implicitly.
- Store vault files with mode `0600` and encrypt profile contents with
  AES-256-GCM.
- Keep the vault master key in Secret Service, not alongside encrypted data.
- Copy Chrome databases to private temporary files and remove them after use.
- Do not print cookie values unless the user explicitly exports them.
- Treat unsupported encryption/keyring states as explicit errors.

## Deferred work

TUI editing, request history, profile diff, expiration notifications, Go/Rust
exporters, browser extension synchronization, Firefox, macOS, and Windows are
post-MVP features.
