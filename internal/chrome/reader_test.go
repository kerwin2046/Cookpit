package chrome

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

type recordingDecrypter struct {
	host      string
	dbVersion int
	value     string
}

func (d *recordingDecrypter) Decrypt(_ []byte, host string, dbVersion int) (string, error) {
	d.host = host
	d.dbVersion = dbVersion
	return d.value, nil
}

func TestReadCookiesReturnsOnlyCookiesApplicableToHost(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "Cookies")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TABLE meta (key LONGVARCHAR NOT NULL UNIQUE PRIMARY KEY, value LONGVARCHAR);
		INSERT INTO meta(key, value) VALUES ('version', '24');
		CREATE TABLE cookies (
			host_key TEXT NOT NULL,
			name TEXT NOT NULL,
			value TEXT NOT NULL,
			encrypted_value BLOB NOT NULL,
			path TEXT NOT NULL,
			expires_utc INTEGER NOT NULL,
			is_secure INTEGER NOT NULL,
			is_httponly INTEGER NOT NULL,
			has_expires INTEGER NOT NULL,
			samesite INTEGER NOT NULL
		);
		INSERT INTO cookies VALUES
			('.example.com', 'parent', 'plain', X'', '/', 0, 1, 1, 0, 1),
			('api.example.com', 'host', '', X'76313000', '/v1', 13443235200000000, 0, 0, 1, -1),
			('badexample.com', 'suffix', 'wrong', X'', '/', 0, 0, 0, 0, -1),
			('.other.com', 'other', 'wrong', X'', '/', 0, 0, 0, 0, -1);
	`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	decrypter := &recordingDecrypter{value: "decrypted"}
	profile := Profile{Browser: "Chrome", Application: "chrome", Name: "Default", CookiesPath: dbPath}
	got, err := ReadCookies(context.Background(), profile, "api.example.com", decrypter)
	if err != nil {
		t.Fatalf("ReadCookies: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("got %d cookies, want 2: %#v", len(got), got)
	}
	if got[0].Name != "parent" || got[0].Value != "plain" || got[0].HostOnly {
		t.Errorf("parent cookie = %#v", got[0])
	}
	if got[1].Name != "host" || got[1].Value != "decrypted" || !got[1].HostOnly {
		t.Errorf("host cookie = %#v", got[1])
	}
	if decrypter.host != "api.example.com" || decrypter.dbVersion != 24 {
		t.Errorf("decrypter called with host=%q version=%d", decrypter.host, decrypter.dbVersion)
	}
	wantExpiry := time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC)
	if got[1].Expires == nil || !got[1].Expires.Equal(wantExpiry) {
		t.Errorf("expiry = %v, want %v", got[1].Expires, wantExpiry)
	}

	info, err := os.Stat(dbPath)
	if err != nil || info.Size() == 0 {
		t.Fatalf("source database was modified: info=%v err=%v", info, err)
	}
}
