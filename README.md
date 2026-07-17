# Cookiex

Cookiex bridges cookies from an authenticated local Chrome/Chromium profile
into terminal HTTP workflows.

```bash
cookiex import github.com --profile work
cookiex show work
cookiex play https://github.com/settings --profile work
cookiex send GET https://github.com/settings --profile work
cookiex export work --format curl
cookiex sync work
cookiex profiles
```

Cookiex only reads cookies already stored by your local browser. It does not
log in to sites, bypass authentication, or guarantee that every browser session
can be replayed. Some sites also require CSRF tokens, local storage, browser
headers, TLS fingerprints, IP binding, or device attestation.

## Current scope

- Linux
- Google Chrome and Chromium
- Domain-scoped imports
- Explicit, encrypted cookie snapshots
- Built-in HTTP request sender and `play` playground
- Fullscreen `ui` Cookie Playground (TUI)
- curl, Python requests, JavaScript fetch, axios, HTTPie, and curl_cffi export
- Profile default headers (encrypted with cookies)
- Multiple Cookiex profiles

Cookie editing, TUI, profile diff, expiry notifications, Firefox, macOS, and
Windows are not implemented yet.

## Build

The project uses Go's toolchain management and currently targets Go 1.25.

```bash
go build -o ./bin/cookiex ./cmd/cookiex
./bin/cookiex --help
```

## Usage

Import cookies that Chrome would send to one host:

```bash
cookiex import app.example.com --profile work
```

Existing profiles are protected. Overwrite only with:

```bash
cookiex import app.example.com --profile work --force
```

Inspect a snapshot without printing secrets:

```bash
cookiex show work
cookiex show work --values
```

Play an authenticated request and inspect the response plus client snippets:

```bash
cookiex play https://app.example.com/api/me --profile work
cookiex play https://app.example.com/api/items \
  --profile work \
  -X POST \
  -H 'Content-Type=application/json' \
  -H 'x-vis-domain=www.compamed-tradefair.com' \
  -d '{"name":"demo"}' \
  --snippet none
```

Open the fullscreen playground:

```bash
cookiex ui
cookiex ui 'https://www.compamed-tradefair.com/vis-api/vis/v1/en/directory/a' --profile work
```

In the TUI:

- `←/→` change profile or method
- `a` / `e` / `d` add, edit, delete headers
- `p` mark a header as profile default `[P]`
- `Ctrl+Enter` send
- `Ctrl+S` save enabled `[P]` headers into the encrypted profile
- `1/2/3` switch Response / Request / Code
- `[` / `]` cycle code formats

When more than one Chrome profile exists, Cookiex asks once and remembers the
selected browser profile. Override it when needed:

```bash
cookiex import app.example.com \
  --profile personal \
  --chrome-profile "Chrome:Profile 2"
```

Send a request:

```bash
cookiex send POST https://app.example.com/api/items \
  --profile work \
  -H 'Content-Type=application/json' \
  -d '{"name":"demo"}'
```

Export runnable code:

```bash
cookiex export work --format curl
cookiex export work --format python --url https://app.example.com/api/me
cookiex export work --format javascript
```

Refresh an existing snapshot:

```bash
cookiex sync work
```

## Storage and safety

- Imports are limited to a target host; there is no implicit all-sites export.
- Cookie values are not shown by `import` or `profiles`.
- Profile files are encrypted with AES-256-GCM and written with mode `0600`.
- The random vault key is stored in Linux Secret Service.
- Chrome's cookie database is copied to a private temporary directory and is
  never modified.
- Chrome `v10` and `v11` OSCrypt values are supported, including the database
  version 24 domain-hash prefix.
- A locked or unavailable desktop keyring is reported as an error. Headless
  environments need a working Secret Service session.

Exports intentionally contain live credentials. Treat generated output as a
secret: do not paste it into issue trackers, shell history, or source control.

## Development

```bash
go test ./...
go vet ./...
go test -race ./...
```

The validated design and implementation plan are in `docs/plans/`.
