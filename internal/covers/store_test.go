package covers_test

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/aydam2728/bookplate/internal/covers"
	"github.com/aydam2728/bookplate/internal/platform/database"
	"github.com/aydam2728/bookplate/internal/platform/scope"
)

func newStore(t *testing.T) (*covers.Store, *sql.DB, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := database.Open(t.Context(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := database.Migrate(t.Context(), db, database.Migrations()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return covers.NewStore(db, dir), db, dir
}

func household(t *testing.T, db *sql.DB, name string) scope.Caller {
	t.Helper()
	const now = "2026-01-01T00:00:00Z"
	if _, err := db.ExecContext(t.Context(),
		`INSERT INTO library (id, name, created_at) VALUES (?, ?, ?)`,
		name+"-lib", name, now); err != nil {
		t.Fatalf("creating the library: %v", err)
	}
	return scope.Caller{UserID: name + "-user", LibraryID: name + "-lib", Role: scope.RoleOwner}
}

func TestSave_ThenRead_RoundTrips(t *testing.T) {
	store, db, _ := newStore(t)
	owner := household(t, db, "alpha")

	assetID, err := store.Save(t.Context(), owner, pngOf(t, 80, 120), "https://example.org/c.png", "Example")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, mime, err := store.Read(t.Context(), owner, assetID)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if mime != "image/jpeg" {
		t.Errorf("mime = %q, want image/jpeg", mime)
	}
	if len(data) == 0 || data[0] != 0xff || data[1] != 0xd8 {
		t.Error("what came back is not the re-encoded JPEG")
	}
}

func TestSave_NamesTheFileAfterItsContent(t *testing.T) {
	store, db, dir := newStore(t)
	owner := household(t, db, "alpha")

	if _, err := store.Save(t.Context(), owner, pngOf(t, 40, 60), "https://evil/../../etc/passwd", ""); err != nil {
		t.Fatalf("Save: %v", err)
	}

	var found []string
	err := filepath.Walk(filepath.Join(dir, covers.Dir), func(p string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			found = append(found, filepath.Base(p))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the cover directory: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("found %d files, want 1: %v", len(found), found)
	}
	if filepath.Ext(found[0]) != ".jpg" || len(found[0]) != 64+4 {
		t.Errorf("file is named %q, want a 64 character digest plus .jpg: a name "+
			"taken from the source URL is a path traversal", found[0])
	}
}

func TestSave_TheSameImageTwice_ReusesTheRecord(t *testing.T) {
	store, db, _ := newStore(t)
	owner := household(t, db, "alpha")
	raw := pngOf(t, 50, 75)

	first, err := store.Save(t.Context(), owner, raw, "", "")
	if err != nil {
		t.Fatalf("first Save: %v", err)
	}
	second, err := store.Save(t.Context(), owner, raw, "", "")
	if err != nil {
		t.Fatalf("second Save: %v", err)
	}
	if first != second {
		t.Error("the same image was stored twice; a household cataloguing a " +
			"series would keep forty copies of one publisher logo")
	}
}

func TestSave_WhenTheCallerIsAReader_Refuses(t *testing.T) {
	store, db, _ := newStore(t)
	reader := household(t, db, "alpha")
	reader.Role = scope.RoleReader

	if _, err := store.Save(t.Context(), reader, pngOf(t, 10, 10), "", ""); !errors.Is(err, scope.ErrForbidden) {
		t.Errorf("a reader stored a cover, err = %v", err)
	}
}

func TestRead_DoesNotServeAnotherHouseholdsCover(t *testing.T) {
	store, db, _ := newStore(t)
	alpha := household(t, db, "alpha")
	beta := household(t, db, "beta")

	assetID, err := store.Save(t.Context(), alpha, pngOf(t, 30, 45), "", "")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, _, err := store.Read(t.Context(), beta, assetID); !errors.Is(err, covers.ErrNotFound) {
		t.Errorf("another household read the cover, err = %v", err)
	}
}

func TestRead_WhenTheCallerIsAnonymous_Refuses(t *testing.T) {
	store, db, _ := newStore(t)
	owner := household(t, db, "alpha")
	assetID, err := store.Save(t.Context(), owner, pngOf(t, 30, 45), "", "")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, _, err := store.Read(t.Context(), scope.Caller{}, assetID); !errors.Is(err, scope.ErrForbidden) {
		t.Errorf("an anonymous caller read a cover, err = %v", err)
	}
}

func TestSave_RefusesABomb(t *testing.T) {
	store, db, _ := newStore(t)
	owner := household(t, db, "alpha")

	if _, err := store.Save(t.Context(), owner, bombHeader(50000, 50000), "", ""); !errors.Is(err, covers.ErrTooLarge) {
		t.Errorf("a decompression bomb reached the disk, err = %v", err)
	}
}

func TestCredit(t *testing.T) {
	for _, c := range []struct{ target, want string }{
		{"https://covers.openlibrary.org/b/id/14314874-L.jpg", "Open Library"},
		{"https://books.google.com/books/content?id=x&printsec=frontcover", "Google Books"},
		{"https://inventaire.io/img/entities/abc", "Inventaire"},
		// A host nobody has named is credited by its own name. The stored
		// credit then says something true rather than something convenient.
		{"https://example.org/cover.jpg", "example.org"},
		{"not a url at all", "unknown"},
		{"", "unknown"},
	} {
		if got := covers.Credit(c.target); got != c.want {
			t.Errorf("Credit(%q) = %q, want %q", c.target, got, c.want)
		}
	}
}
