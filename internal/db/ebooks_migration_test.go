package db_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"discodrive/internal/db"
)

// TestMigration022Ebooks verifies that migration 000022_ebooks applies cleanly and that the
// sqlc-generated queries for ebook_settings, books, book_authors, and book_tags work end-to-end.
func TestMigration022Ebooks(t *testing.T) {
	ctx := context.Background()
	pgC, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("kf"), tcpostgres.WithUsername("kf"), tcpostgres.WithPassword("kf"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp").WithStartupTimeout(60*time.Second)))
	if err != nil {
		t.Skipf("need Docker: %v", err)
	}
	t.Cleanup(func() { _ = pgC.Terminate(ctx) })

	dsn, _ := pgC.ConnectionString(ctx, "sslmode=disable")
	if err := db.MigrateUp(dsn); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool: %v", err)
	}
	t.Cleanup(pool.Close)

	q := db.New(pool)

	// Create tenant and user.
	tenant, err := q.CreateTenant(ctx, "test-tenant")
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	u, err := q.CreateUser(ctx, db.CreateUserParams{
		TenantID:     tenant.ID,
		Email:        "ebooks@test.local",
		PasswordHash: "x",
		Role:         "user",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// --- ebook_settings ---

	// Absent row → ErrNoRows.
	_, err = q.GetEbookSettings(ctx, u.ID)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected pgx.ErrNoRows for absent ebook_settings row, got: %v", err)
	}

	// Upsert with enabled=false (no folder set yet).
	es, err := q.UpsertEbookSettings(ctx, db.UpsertEbookSettingsParams{
		UserID:  u.ID,
		Enabled: false,
		// FolderNodeID: zero-value pgtype.UUID (Valid=false → NULL in DB)
	})
	if err != nil {
		t.Fatalf("UpsertEbookSettings: %v", err)
	}
	if es.Enabled {
		t.Fatalf("expected enabled=false after upsert, got true")
	}
	if db.UUIDString(es.UserID) != db.UUIDString(u.ID) {
		t.Fatalf("ebook_settings user_id mismatch: got %s want %s", db.UUIDString(es.UserID), db.UUIDString(u.ID))
	}

	// Read-back persists.
	got, err := q.GetEbookSettings(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetEbookSettings: %v", err)
	}
	if got.Enabled {
		t.Fatalf("expected enabled=false on read-back")
	}

	// SetEbookCredentials sets password_cipher and api_key.
	if err := q.SetEbookCredentials(ctx, db.SetEbookCredentialsParams{
		UserID:         u.ID,
		PasswordCipher: pgtype.Text{String: "encrypted-blob", Valid: true},
		ApiKey:         pgtype.Text{String: "testkey-abc123", Valid: true},
	}); err != nil {
		t.Fatalf("SetEbookCredentials: %v", err)
	}

	// GetEbookSettingsByApiKey finds the row by api_key.
	byKey, err := q.GetEbookSettingsByApiKey(ctx, pgtype.Text{String: "testkey-abc123", Valid: true})
	if err != nil {
		t.Fatalf("GetEbookSettingsByApiKey: %v", err)
	}
	if db.UUIDString(byKey.UserID) != db.UUIDString(u.ID) {
		t.Fatalf("GetEbookSettingsByApiKey user_id mismatch")
	}

	// Enable and set a folder node so EnabledEbookUsers returns the row.
	// Create a directory node to serve as the books folder.
	folderNode, err := q.CreateNode(ctx, db.CreateNodeParams{
		UserID: u.ID,
		Name:   "Books",
		IsDir:  true,
	})
	if err != nil {
		t.Fatalf("CreateNode (folder): %v", err)
	}

	es, err = q.UpsertEbookSettings(ctx, db.UpsertEbookSettingsParams{
		UserID:        u.ID,
		Enabled:       true,
		FolderNodeID:  folderNode.ID,
	})
	if err != nil {
		t.Fatalf("UpsertEbookSettings (enable): %v", err)
	}
	if !es.Enabled {
		t.Fatalf("expected enabled=true after second upsert")
	}

	enabled, err := q.EnabledEbookUsers(ctx)
	if err != nil {
		t.Fatalf("EnabledEbookUsers: %v", err)
	}
	if len(enabled) != 1 {
		t.Fatalf("EnabledEbookUsers: expected 1 row, got %d", len(enabled))
	}
	if db.UUIDString(enabled[0].UserID) != db.UUIDString(u.ID) {
		t.Fatalf("EnabledEbookUsers: unexpected user_id")
	}

	// ClearEbookCredentials nulls credentials.
	if err := q.ClearEbookCredentials(ctx, u.ID); err != nil {
		t.Fatalf("ClearEbookCredentials: %v", err)
	}
	cleared, err := q.GetEbookSettings(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetEbookSettings after clear: %v", err)
	}
	if cleared.PasswordCipher.Valid {
		t.Fatalf("expected password_cipher=NULL after clear, still set")
	}
	if cleared.ApiKey.Valid {
		t.Fatalf("expected api_key=NULL after clear, still set")
	}

	// --- books ---

	// Create a file node for the book.
	fileNode, err := q.CreateNode(ctx, db.CreateNodeParams{
		UserID: u.ID,
		Name:   "test-book.epub",
		IsDir:  false,
		Size:   pgtype.Int8{Int64: 204800, Valid: true},
	})
	if err != nil {
		t.Fatalf("CreateNode (book file): %v", err)
	}

	book, err := q.UpsertBook(ctx, db.UpsertBookParams{
		UserID:      u.ID,
		NodeID:      fileNode.ID,
		Title:       "The Go Programming Language",
		SortTitle:   "go programming language",
		Language:    pgtype.Text{String: "en", Valid: true},
		Isbn:        pgtype.Text{String: "978-0-13-419562-1", Valid: true},
		Description: pgtype.Text{String: "A comprehensive guide to Go.", Valid: true},
		Publisher:   pgtype.Text{String: "Addison-Wesley", Valid: true},
		Series:      pgtype.Text{},
		Format:      "epub",
		ContentType: "application/epub+zip",
		Size:        pgtype.Int8{Int64: 204800, Valid: true},
	})
	if err != nil {
		t.Fatalf("UpsertBook: %v", err)
	}
	if book.Title != "The Go Programming Language" {
		t.Fatalf("UpsertBook: title mismatch, got %q", book.Title)
	}

	// Upsert again (same node_id) → ON CONFLICT DO UPDATE, updated_at changes.
	book2, err := q.UpsertBook(ctx, db.UpsertBookParams{
		UserID:      u.ID,
		NodeID:      fileNode.ID,
		Title:       "The Go Programming Language (2nd Ed)",
		SortTitle:   "go programming language 2nd ed",
		Format:      "epub",
		ContentType: "application/epub+zip",
	})
	if err != nil {
		t.Fatalf("UpsertBook (conflict): %v", err)
	}
	if db.UUIDString(book2.ID) != db.UUIDString(book.ID) {
		t.Fatalf("UpsertBook: expected same book id on conflict, got different")
	}
	if book2.Title != "The Go Programming Language (2nd Ed)" {
		t.Fatalf("UpsertBook: title not updated on conflict, got %q", book2.Title)
	}

	// GetBookByNode retrieves by node id.
	byNode, err := q.GetBookByNode(ctx, fileNode.ID)
	if err != nil {
		t.Fatalf("GetBookByNode: %v", err)
	}
	if db.UUIDString(byNode.ID) != db.UUIDString(book.ID) {
		t.Fatalf("GetBookByNode: id mismatch")
	}

	// SetBookCoverPath.
	if err := q.SetBookCoverPath(ctx, db.SetBookCoverPathParams{
		ID:        book.ID,
		CoverPath: pgtype.Text{String: ".covers/ebooks/test.jpg", Valid: true},
	}); err != nil {
		t.Fatalf("SetBookCoverPath: %v", err)
	}

	// --- book_authors ---

	if err := q.InsertBookAuthor(ctx, db.InsertBookAuthorParams{
		BookID:   book.ID,
		Name:     "Alan A. A. Donovan",
		SortName: "donovan alan",
	}); err != nil {
		t.Fatalf("InsertBookAuthor: %v", err)
	}
	if err := q.InsertBookAuthor(ctx, db.InsertBookAuthorParams{
		BookID:   book.ID,
		Name:     "Brian W. Kernighan",
		SortName: "kernighan brian",
	}); err != nil {
		t.Fatalf("InsertBookAuthor (2): %v", err)
	}

	authors, err := q.BookAuthors(ctx, book.ID)
	if err != nil {
		t.Fatalf("BookAuthors: %v", err)
	}
	if len(authors) != 2 {
		t.Fatalf("BookAuthors: expected 2, got %d", len(authors))
	}

	// --- book_tags ---

	if err := q.InsertBookTag(ctx, db.InsertBookTagParams{BookID: book.ID, Tag: "programming"}); err != nil {
		t.Fatalf("InsertBookTag: %v", err)
	}
	if err := q.InsertBookTag(ctx, db.InsertBookTagParams{BookID: book.ID, Tag: "golang"}); err != nil {
		t.Fatalf("InsertBookTag (2): %v", err)
	}
	// Duplicate insert → ON CONFLICT DO NOTHING (must not error).
	if err := q.InsertBookTag(ctx, db.InsertBookTagParams{BookID: book.ID, Tag: "golang"}); err != nil {
		t.Fatalf("InsertBookTag (duplicate): %v", err)
	}

	tags, err := q.BookTags(ctx, book.ID)
	if err != nil {
		t.Fatalf("BookTags: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("BookTags: expected 2, got %d", len(tags))
	}

	// --- Accessible scope queries ---

	// AccessibleBook by id (own book).
	aBook, err := q.AccessibleBook(ctx, db.AccessibleBookParams{
		UserID: u.ID,
		ID:     book.ID,
	})
	if err != nil {
		t.Fatalf("AccessibleBook: %v", err)
	}
	if db.UUIDString(aBook.ID) != db.UUIDString(book.ID) {
		t.Fatalf("AccessibleBook: id mismatch")
	}

	// AccessibleBooksAll returns our book.
	allBooks, err := q.AccessibleBooksAll(ctx, db.AccessibleBooksAllParams{
		UserID: u.ID,
		Limit:  10,
		Offset: 0,
	})
	if err != nil {
		t.Fatalf("AccessibleBooksAll: %v", err)
	}
	if len(allBooks) != 1 {
		t.Fatalf("AccessibleBooksAll: expected 1, got %d", len(allBooks))
	}

	// CountAccessibleBooks.
	cnt, err := q.CountAccessibleBooks(ctx, u.ID)
	if err != nil {
		t.Fatalf("CountAccessibleBooks: %v", err)
	}
	if cnt != 1 {
		t.Fatalf("CountAccessibleBooks: expected 1, got %d", cnt)
	}

	// AccessibleBooksNewest.
	newest, err := q.AccessibleBooksNewest(ctx, db.AccessibleBooksNewestParams{
		UserID: u.ID,
		Limit:  5,
		Offset: 0,
	})
	if err != nil {
		t.Fatalf("AccessibleBooksNewest: %v", err)
	}
	if len(newest) != 1 {
		t.Fatalf("AccessibleBooksNewest: expected 1, got %d", len(newest))
	}

	// AccessibleAuthors returns the 2 authors.
	accAuthors, err := q.AccessibleAuthors(ctx, u.ID)
	if err != nil {
		t.Fatalf("AccessibleAuthors: %v", err)
	}
	if len(accAuthors) != 2 {
		t.Fatalf("AccessibleAuthors: expected 2, got %d", len(accAuthors))
	}

	// AccessibleBookGenres returns 2 tags.
	genres, err := q.AccessibleBookGenres(ctx, u.ID)
	if err != nil {
		t.Fatalf("AccessibleBookGenres: %v", err)
	}
	if len(genres) != 2 {
		t.Fatalf("AccessibleBookGenres: expected 2, got %d", len(genres))
	}

	// SearchAccessibleBooks by title substring.
	results, err := q.SearchAccessibleBooks(ctx, db.SearchAccessibleBooksParams{
		UserID:  u.ID,
		Column2: pgtype.Text{String: "Go Programming", Valid: true},
		Limit:   10,
		Offset:  0,
	})
	if err != nil {
		t.Fatalf("SearchAccessibleBooks: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("SearchAccessibleBooks: expected 1 result, got %d", len(results))
	}

	// --- Cleanup queries ---

	// ClearBookAuthors removes all authors for the book.
	if err := q.ClearBookAuthors(ctx, book.ID); err != nil {
		t.Fatalf("ClearBookAuthors: %v", err)
	}
	afterClear, err := q.BookAuthors(ctx, book.ID)
	if err != nil {
		t.Fatalf("BookAuthors after clear: %v", err)
	}
	if len(afterClear) != 0 {
		t.Fatalf("expected 0 authors after ClearBookAuthors, got %d", len(afterClear))
	}

	// DeleteBookByNode removes the book.
	if err := q.DeleteBookByNode(ctx, fileNode.ID); err != nil {
		t.Fatalf("DeleteBookByNode: %v", err)
	}
	_, err = q.GetBookByNode(ctx, fileNode.ID)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected ErrNoRows after DeleteBookByNode, got: %v", err)
	}
}
