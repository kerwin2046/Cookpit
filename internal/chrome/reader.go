package chrome

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	cookiemodel "cookiex/internal/cookie"

	_ "modernc.org/sqlite"
)

const chromeToUnixEpochMicros int64 = 11644473600000000

// CookieDecrypter decrypts a value from Chrome's encrypted_value column.
type CookieDecrypter interface {
	Decrypt(encrypted []byte, host string, dbVersion int) (string, error)
}

// ReadCookies copies a Chrome database and returns unexpired cookies that may
// be sent to targetHost.
func ReadCookies(ctx context.Context, profile Profile, targetHost string, decrypter CookieDecrypter) ([]cookiemodel.Cookie, error) {
	host, err := cookiemodel.NormalizeHost(targetHost)
	if err != nil {
		return nil, err
	}

	tempDir, err := os.MkdirTemp("", "cookiex-chrome-*")
	if err != nil {
		return nil, fmt.Errorf("create private Chrome database directory: %w", err)
	}
	defer os.RemoveAll(tempDir)
	if err := os.Chmod(tempDir, 0o700); err != nil {
		return nil, fmt.Errorf("protect Chrome database directory: %w", err)
	}

	tempDB := filepath.Join(tempDir, "Cookies")
	if err := copySQLiteDatabase(profile.CookiesPath, tempDB); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", tempDB)
	if err != nil {
		return nil, fmt.Errorf("open copied Chrome cookie database: %w", err)
	}
	defer db.Close()

	var dbVersion int
	if err := db.QueryRowContext(ctx, "SELECT value FROM meta WHERE key = 'version'").Scan(&dbVersion); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("read Chrome cookie database version: %w", err)
	}

	rows, err := db.QueryContext(ctx, `
		SELECT host_key, name, value, encrypted_value, path, expires_utc,
		       is_secure, is_httponly, has_expires, samesite
		FROM cookies
		ORDER BY host_key, name, path
	`)
	if err != nil {
		return nil, fmt.Errorf("query Chrome cookies: %w", err)
	}
	defer rows.Close()

	now := time.Now()
	var result []cookiemodel.Cookie
	for rows.Next() {
		var (
			domain, name, plainValue, path string
			encryptedValue                 []byte
			expiresMicros                  int64
			secure, httpOnly, hasExpires   bool
			sameSite                       int
		)
		if err := rows.Scan(
			&domain, &name, &plainValue, &encryptedValue, &path, &expiresMicros,
			&secure, &httpOnly, &hasExpires, &sameSite,
		); err != nil {
			return nil, fmt.Errorf("scan Chrome cookie: %w", err)
		}

		var expires *time.Time
		if hasExpires && expiresMicros > 0 {
			value := chromeTime(expiresMicros)
			expires = &value
		}
		item := cookiemodel.Cookie{
			Name:     name,
			Value:    plainValue,
			Domain:   domain,
			Path:     path,
			Secure:   secure,
			HTTPOnly: httpOnly,
			SameSite: sameSite,
			HostOnly: !strings.HasPrefix(domain, "."),
			Expires:  expires,
		}
		if !item.Matches(cookiemodel.RequestContext{
			Host:   host,
			Path:   path,
			Secure: true,
			Now:    now,
		}) {
			continue
		}

		if item.Value == "" && len(encryptedValue) > 0 {
			if decrypter == nil {
				return nil, fmt.Errorf("cookie %q for %q is encrypted but no decrypter is configured", name, domain)
			}
			item.Value, err = decrypter.Decrypt(encryptedValue, domain, dbVersion)
			if err != nil {
				return nil, fmt.Errorf("decrypt cookie %q for %q: %w", name, domain, err)
			}
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Chrome cookies: %w", err)
	}
	return result, nil
}

func chromeTime(microseconds int64) time.Time {
	return time.UnixMicro(microseconds - chromeToUnixEpochMicros).UTC()
}

func copySQLiteDatabase(source, destination string) error {
	if err := copyPrivateFile(source, destination); err != nil {
		return fmt.Errorf("copy Chrome cookie database: %w", err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := copyPrivateFile(source+suffix, destination+suffix); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("copy Chrome cookie database%s: %w", suffix, err)
		}
	}
	return nil
}

func copyPrivateFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()

	output, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
