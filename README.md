# Cookiex

Cookiex bridges cookies from an authenticated local Chrome/Chromium profile
into terminal HTTP workflows.

```bash
cookiex import github.com --profile work
cookiex show work
cookiex diff work
cookiex play https://github.com/settings --profile work
cookiex send GET https://github.com/settings --profile work
cookiex export work --format curl
cookiex sync work
cookiex profiles
cookiex ui
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
- Snapshot vs Chrome profile diff

Cookie editing, expiry notifications, Firefox, macOS, and Windows are not
implemented yet.

## Install

Requires Go 1.25+ (the module uses Go toolchain management).

```bash
go build -o ./bin/cookiex ./cmd/cookiex
./bin/cookiex --help
```

Or install into your `GOBIN`:

```bash
go install ./cmd/cookiex
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

When more than one Chrome profile exists, Cookiex asks once and remembers the
selected browser profile. Override it when needed:

```bash
cookiex import app.example.com \
  --profile personal \
  --chrome-profile "Chrome:Profile 2"
```

Inspect a snapshot without printing secrets:

```bash
cookiex show work
cookiex show work --values
cookiex profiles
```

Compare a snapshot to the current Chrome cookies before refreshing:

```bash
cookiex diff work
cookiex diff work --values
```

Refresh an existing snapshot:

```bash
cookiex sync work
```

Send a request:

```bash
cookiex send POST https://app.example.com/api/items \
  --profile work \
  -H 'Content-Type=application/json' \
  -d '{"name":"demo"}'
```

Play an authenticated request and inspect the response plus client snippets:

```bash
cookiex play https://app.example.com/api/me --profile work
cookiex play https://app.example.com/api/items \
  --profile work \
  -X POST \
  -H 'Content-Type=application/json' \
  -d '{"name":"demo"}' \
  --snippet all
```

`--snippet` accepts `curl` (default), `all`, `none`, or a comma list such as
`python,axios`.

Open the fullscreen playground:

```bash
cookiex ui
cookiex ui 'https://app.example.com/api/me' --profile work
```

In the TUI:

- `Tab` / `Shift+Tab` move focus
- `←/→` change profile, method, or result tabs
- `Space` enable/disable a header
- `a` / `e` / `d` add, edit, delete headers
- `p` mark a header as profile default `[P]`
- `Enter` send (in Body, Enter inserts a newline; Tab away then Enter to send)
- `Ctrl+R` refresh profile cookies from Chrome
- `Ctrl+H` cycle request history
- `Ctrl+P` save current form as a named preset
- `Ctrl+O` cycle named presets
- `Ctrl+S` save enabled `[P]` headers into the encrypted profile
- `1/2/3` switch Response / Request / Code
- `[` / `]` cycle code formats
- `q` quit

Export runnable code:

```bash
cookiex export work --format curl
cookiex export work --format python --url https://app.example.com/api/me
cookiex export work --format javascript
cookiex export work --format curl_cffi
```

Supported formats: `curl`, `python`, `javascript`, `axios`, `httpie`,
`curl_cffi`.

## Storage and safety

- Profiles live under `$XDG_DATA_HOME/cookiex/profiles`
  (default `~/.local/share/cookiex/profiles`).
- Playground history and presets live in
  `$XDG_DATA_HOME/cookiex/playground.json` (mode `0600`; Cookie headers are
  never written).
- Chrome profile selection is remembered in `$XDG_CONFIG_HOME/cookiex/selection.json`.
- Imports are limited to a target host; there is no implicit all-sites export.
- Cookie values are not shown by `import`, `profiles`, or `diff` unless
  `--values` is set.
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

## License

MIT. See [LICENSE](LICENSE).
