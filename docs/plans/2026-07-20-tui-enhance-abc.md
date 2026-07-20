# TUI Enhance A→B→C Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans or superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Enhance Cookiex Playground with (A) matched-cookie visibility + in-TUI sync, (B) request history and named presets, and (C) host-derived header auto-fill with `{{host}}` templates—shipped as small, tested, MIT-friendly commits.

**Architecture:** Keep pure logic in testable packages (`request` matching helpers, new `internal/history` and header template helpers). Inject a `ProfileSyncer` into the TUI so Bubble Tea stays thin. Persist history/presets under `$XDG_DATA_HOME/cookiex/` as JSON (mode `0600`), never logging cookie values. Expand header templates at send time only.

**Tech Stack:** Go, Bubble Tea, existing vault/request/chrome packages, XDG data dir, TDD.

**Open-source norms:** Conventional commits (`feat:`, `test:`, `docs:`), tests before code, no secrets in fixtures, README updates per feature, YAGNI (no Firefox/macOS in this plan).

**Prerequisite:** Enter-to-send keybinding (already in working tree) lands in the first commit on the feature branch.

---

### Task 1: Prerequisite — commit Enter-to-send

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `README.md`
- Modify: `docs/plans/2026-07-17-cookiex-tui.md`

**Step 1: Verify tests still pass**

Run: `go test ./internal/tui/ ./...`
Expected: PASS

**Step 2: Commit**

```bash
git add internal/tui/model.go README.md docs/plans/2026-07-17-cookiex-tui.md
git commit -m "$(cat <<'EOF'
feat(tui): send requests with Enter instead of Ctrl+Enter

Ctrl+Enter is unreliable in VS Code integrated terminals; Enter
sends from non-body focus while Body still uses Enter for newlines.
EOF
)"
```

---

### Task 2: A1 — matched cookie names helper (pure logic)

**Files:**
- Modify: `internal/request/runner.go` (export/reuse `MatchingCookies`; add name helper if needed)
- Create: `internal/request/match_summary.go` (optional thin wrapper)
- Test: `internal/request/runner_test.go` or `internal/request/match_summary_test.go`

**Step 1: Write the failing test**

```go
func TestMatchedCookieNames(t *testing.T) {
	target, err := url.Parse("https://www.example.com/api")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	cookies := []cookiemodel.Cookie{
		{Name: "session", Domain: ".example.com", Path: "/", Value: "secret"},
		{Name: "other", Domain: ".other.com", Path: "/", Value: "x"},
		{Name: "expired", Domain: ".example.com", Path: "/", Value: "x", Expires: ptr(now.Add(-time.Hour))},
	}
	names := MatchedCookieNames(target, cookies, now)
	if len(names) != 1 || names[0] != "session" {
		t.Fatalf("names = %#v", names)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/request/ -run TestMatchedCookieNames -v`
Expected: FAIL — `MatchedCookieNames` undefined

**Step 3: Minimal implementation**

```go
func MatchedCookieNames(target *url.URL, cookies []cookiemodel.Cookie, now time.Time) []string {
	matched := MatchingCookies(target, cookies, now)
	names := make([]string, len(matched))
	for i, c := range matched {
		names[i] = c.Name
	}
	sort.Strings(names)
	return names
}
```

**Step 4: Run test — PASS**

**Step 5: Commit**

```bash
git commit -m "feat(request): list matched cookie names without values"
```

---

### Task 3: A2 — Request tab shows matched cookie names

**Files:**
- Modify: `internal/tui/model.go` (`requestContent`, status after send)
- Test: `internal/tui/logic_test.go` or `model_test.go` — pure helper for request summary line

**Step 1: Failing test for summary formatter**

```go
func TestFormatMatchedCookiesLine(t *testing.T) {
	got := FormatMatchedCookiesLine([]string{"a", "b"})
	want := "Cookie: [redacted — 2 matched: a, b]"
	if got != want {
		t.Fatalf("got %q", got)
	}
	if FormatMatchedCookiesLine(nil) != "Cookie: [redacted — 0 matched]" {
		t.Fatal("empty")
	}
}
```

**Step 2: Implement + wire into `requestContent` using URL + profile cookies**

Use `request.MatchedCookieNames` against current/last URL. Cap display at ~12 names then `…`.

**Step 3: Update status after successful send**

Example: `200 OK · 824ms · 3 cookies`

**Step 4: Commit**

```bash
git commit -m "feat(tui): show matched cookie names on Request tab"
```

---

### Task 4: A3 — ProfileSyncer + Ctrl+R sync in TUI

**Files:**
- Modify: `internal/tui/model.go`, `internal/tui/run.go`
- Modify: `internal/cli/root.go` (`newUICommand` wires sync)
- Test: `internal/tui/model_test.go` with fake syncer

**Design:**

```go
type ProfileSyncer interface {
	Sync(ctx context.Context, profile vault.Profile) (vault.Profile, error)
}
```

CLI implements via existing `ReadCookies` + `Save` path (extract shared helper from `newSyncCommand` if clean).

**Keys:** `ctrl+r` syncs current profile; while syncing show `Syncing…`; on success reload headers/cookies and status `synced N → M cookies`.

**Body focus:** allow `ctrl+r` through like `ctrl+s` (do not insert into textarea).

**Step 1: Failing test** — Update with fake syncer updates `m.profile.Cookies`

**Step 2: Implement syncer + keybinding + help/status strings**

**Step 3: README** — document `Ctrl+R` sync

**Step 4: Commit**

```bash
git commit -m "feat(tui): sync profile cookies from Chrome with Ctrl+R"
```

---

### Task 5: B1 — history store (pure)

**Files:**
- Create: `internal/history/store.go`
- Test: `internal/history/store_test.go`

**Model:**

```go
type Entry struct {
	ID        string            `json:"id"`
	SavedAt   time.Time         `json:"saved_at"`
	Profile   string            `json:"profile"`
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      string            `json:"body,omitempty"`
	Name      string            `json:"name,omitempty"` // preset name; empty = history-only
}
```

**Behavior:**
- `AppendHistory(entry)` — prepend, dedupe consecutive identical method+url+profile, cap at 50
- `ListHistory() []Entry` — Name == ""
- `SavePreset(name, entry) error` — Name set, upsert by name
- `ListPresets() []Entry`
- `LoadPreset(name) (Entry, error)`
- `DeletePreset(name) error`
- Atomic write, mode `0600`
- Never store Cookie header values (strip `Cookie` key on save)

**Path:** `$XDG_DATA_HOME/cookiex/history.json` (single file for history + presets, or `history.json` + `presets.json` — prefer one `playground.json` with `{history, presets}`)

**Step 1–4:** TDD as usual

**Step 5: Commit**

```bash
git commit -m "feat(history): persist request history and named presets"
```

---

### Task 6: B2 — wire history into TUI

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/cli/root.go` (pass store path / HistoryStore)
- Test: `internal/tui/*_test.go`
- Modify: `README.md`

**Keys:**
- After successful send → `AppendHistory`
- `ctrl+h` → cycle history backward into form (URL, method, headers, body, profile if present)
- `ctrl+p` → save current form as preset (prompt name via header-style mini editor or use URL path basename — prefer short name input like header editor)
- `ctrl+o` → cycle presets into form

Keep scope tight: name prompt for preset = reuse header-name textinput pattern (single field).

**Step 1–4:** TDD load/apply helpers first, then keys

**Step 5: Commit**

```bash
git commit -m "feat(tui): request history and presets shortcuts"
```

---

### Task 7: C1 — header template expansion

**Files:**
- Modify: `internal/headers/merge.go` or create `internal/headers/template.go`
- Test: `internal/headers/template_test.go`
- Wire: `BuildSpec` / runner callers so send & export expand templates

**Templates:**
- `{{host}}` → URL hostname
- `{{origin}}` → `scheme://host`
- `{{scheme}}` → `http` / `https`

Unknown `{{…}}` left unchanged.

**Step 1–4:** TDD

**Step 5: Commit**

```bash
git commit -m "feat(headers): expand {{host}} templates at send time"
```

---

### Task 8: C2 — auto-fill host-derived headers in TUI

**Files:**
- Modify: `internal/tui/logic.go`
- Test: `internal/tui/logic_test.go`
- Modify: `README.md`

**Rule (documented, not Vis-hardcoded magic beyond one common header):**

```go
// EnsureHostDerivedHeaders adds x-vis-domain={{host}} when:
// - URL parses with a host
// - no existing header named x-vis-domain (case-insensitive)
// Returns updated rows (may be same slice content).
func EnsureHostDerivedHeaders(rawURL string, rows []HeaderRow) []HeaderRow
```

Call on: `New` (after URL set), URL edit completion is hard in textinput — call before `send()` and when cycling focus away from URL (`cycleFocus`), and on profile change if URL already set.

Value stored as literal `{{host}}` so it tracks URL changes; expansion at send.

Also apply same ensure in CLI `play`/`send` when merging headers? Optional — TUI-first; CLI users can `-H 'x-vis-domain={{host}}'`.

**Step 1–4:** TDD

**Step 5: Commit + README**

```bash
git commit -m "feat(tui): auto-add x-vis-domain={{host}} when missing"
```

---

### Task 9: Docs polish + full verification

**Files:**
- Modify: `README.md` (shortcuts table, history path, templates, safety note)
- Modify: `docs/plans/2026-07-20-tui-enhance-abc.md` (mark done)

**Step 1:**

```bash
go test ./...
go vet ./...
go build -o ./bin/cookiex ./cmd/cookiex
```

**Step 2:** Manual smoke in external terminal:

```bash
./bin/cookiex ui
# Enter send → Request tab shows cookie names
# Ctrl+R sync
# Ctrl+H history, Ctrl+P preset, Ctrl+O open preset
# Headers show x-vis-domain={{host}} when absent
```

**Step 3: Final commit if README-only delta**

```bash
git commit -m "docs: document TUI cookie match, sync, history, and host headers"
```

---

## Out of scope (YAGNI)

- Cookie value reveal in UI
- Firefox / macOS / Windows
- Cloud sync of history
- Full collection folders
- Body JSON schema validation

## Execution note

Implement A completely (Tasks 1–4) before B, then C. Stop for review after each letter if the user asks.
