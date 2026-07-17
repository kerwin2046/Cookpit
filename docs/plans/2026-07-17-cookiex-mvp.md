# Cookiex MVP Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a Linux Go CLI that imports domain-scoped cookies from a local Chrome/Chromium profile into an encrypted snapshot, then sends requests or exports runnable code.

**Architecture:** Keep browser discovery/decryption, RFC-style cookie matching, encrypted persistence, request execution, and rendering behind small packages. Commands compose those packages and redact values by default.

**Tech Stack:** Go 1.22, Cobra, `database/sql` with a pure-Go SQLite driver, Linux Secret Service through `go-keyring`, AES-256-GCM, standard `net/http`.

---

### Task 1: Project skeleton and cookie model

**Files:**
- Create: `go.mod`
- Create: `cmd/cookiex/main.go`
- Create: `internal/cookie/cookie.go`
- Test: `internal/cookie/cookie_test.go`

**Steps:**
1. Write tests for canonical host normalization, parent-domain matching without suffix confusion, secure filtering, path matching, and expiry filtering.
2. Run `go test ./internal/cookie` and verify it fails because the model and matcher do not exist.
3. Implement `Cookie`, `RequestContext`, `NormalizeHost`, and `Matches`.
4. Run the package tests and then `go test ./...`.

### Task 2: Chrome profile discovery and SQLite reader

**Files:**
- Create: `internal/chrome/discovery.go`
- Create: `internal/chrome/reader.go`
- Create: `internal/chrome/decrypt.go`
- Test: `internal/chrome/discovery_test.go`
- Test: `internal/chrome/reader_test.go`
- Test: `internal/chrome/decrypt_test.go`

**Steps:**
1. Write table-driven discovery tests using temporary Chrome and Chromium trees.
2. Write a SQLite fixture test proving domain-scoped rows are read from a copied database.
3. Write known-vector tests for legacy plaintext plus Linux `v10`/`v11` AES-CBC decryption and invalid padding.
4. Verify each test fails for the missing behavior.
5. Implement discovery, private temporary database copying, parameterized host filtering, Chrome timestamp conversion, and a decrypter interface.
6. Implement PBKDF2-SHA1 key derivation and AES-CBC decryption; obtain the v11 password through an injected secret provider.
7. Run `go test ./internal/chrome` and the full suite.

### Task 3: Encrypted profile vault

**Files:**
- Create: `internal/vault/vault.go`
- Create: `internal/vault/keyring.go`
- Test: `internal/vault/vault_test.go`

**Steps:**
1. Write tests for encrypted round trips, distinct nonces, tamper detection, profile listing, and `0600` file permissions using an in-memory key provider.
2. Verify the tests fail for missing vault behavior.
3. Implement JSON profile metadata encrypted with AES-256-GCM and atomic file replacement.
4. Implement a Linux Secret Service key provider using `go-keyring`.
5. Run vault tests and the full suite.

### Task 4: Exporters and HTTP runner

**Files:**
- Create: `internal/export/export.go`
- Create: `internal/export/curl.go`
- Create: `internal/export/python.go`
- Create: `internal/export/javascript.go`
- Create: `internal/request/runner.go`
- Test: `internal/export/export_test.go`
- Test: `internal/request/runner_test.go`

**Steps:**
1. Write escaping and output tests for curl, Python requests, and JavaScript fetch.
2. Write an `httptest.Server` test proving only matching cookies are sent.
3. Verify failures before implementation.
4. Implement exporters and a bounded-response HTTP runner with an injectable client.
5. Run package and full tests.

### Task 5: CLI vertical slice

**Files:**
- Create: `internal/cli/root.go`
- Create: `internal/cli/import.go`
- Create: `internal/cli/profiles.go`
- Create: `internal/cli/export.go`
- Create: `internal/cli/send.go`
- Test: `internal/cli/cli_test.go`
- Modify: `cmd/cookiex/main.go`

**Steps:**
1. Write command tests with temporary browser/vault directories and injected dependencies.
2. Verify missing-command failures.
3. Implement `import`, `profiles`, `export`, and `send`; prompt for a Chrome profile only when multiple profiles exist.
4. Keep cookie values redacted outside explicit export/request use.
5. Run CLI tests and the full suite.

### Task 6: Documentation and release verification

**Files:**
- Create: `README.md`
- Create: `.gitignore`
- Create: `LICENSE`

**Steps:**
1. Document installation, the Linux/Chrome-only scope, Secret Service requirement, examples, and replay limitations.
2. Run `gofmt -w .`.
3. Run `go vet ./...`.
4. Run `go test -race ./...`.
5. Run `go build -o ./bin/cookiex ./cmd/cookiex`.
6. Smoke-test `./bin/cookiex --help`.
