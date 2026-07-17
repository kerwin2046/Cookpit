# Cookiex TUI Playground Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add `cookiex ui` as a fullscreen Bubble Tea playground that sends authenticated requests with profile default headers and shows Response / Request / Code tabs.

**Architecture:** Extend encrypted profiles with default headers; share header-merge and request-build logic between CLI and TUI; keep Bubble Tea UI thin over existing runner/export packages.

**Tech Stack:** Go, Bubble Tea, Bubbles, Lip Gloss, existing vault/request/export packages.

---

### Task 1: Profile default headers + merge helper

**Files:**
- Modify: `internal/vault/vault.go`
- Create: `internal/headers/merge.go`
- Test: `internal/vault/vault_test.go`, `internal/headers/merge_test.go`

**Steps:**
1. Add `Headers map[string]string` to `vault.Profile` with round-trip test.
2. Implement `Merge(profileHeaders, requestHeaders)` where request overrides profile.
3. Run `go test ./internal/vault ./internal/headers`.

### Task 2: Wire headers into play/send/export

**Files:**
- Modify: `internal/cli/root.go`
- Test: `internal/cli/cli_test.go`

**Steps:**
1. Fail a play test proving profile headers are sent and `-H` overrides them.
2. Apply merge in play/send/export.
3. Treat `--snippet none` / empty as no snippets; default play snippet to `curl`.

### Task 3: TUI model + command

**Files:**
- Create: `internal/tui/*.go`
- Modify: `internal/cli/root.go`, `README.md`
- Test: `internal/tui/model_test.go`, `internal/cli/cli_test.go`

**Steps:**
1. Test request-spec building and tab cycling without a real terminal.
2. Implement Bubble Tea app: profile, method, URL, headers, body, send, tabs.
3. Add `cookiex ui [url] --profile`.
4. `Ctrl+Enter` send, `Ctrl+S` save profile headers, `q` quit, mouse enabled.
5. Run full `go test ./...` and build binary.
