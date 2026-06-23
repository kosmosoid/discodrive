package ebook

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"discodrive/internal/db"
)

var (
	// ErrNotBook is returned when the node has no corresponding book index entry.
	ErrNotBook = errors.New("ebook: node is not an indexed book")
	// ErrNotInEbookFolder is returned when the node is outside the user's ebook folder.
	ErrNotInEbookFolder = errors.New("ebook: node is not inside the user's ebook folder")
)

// BookMeta is the editable metadata of one book.
type BookMeta struct {
	Title       string   `json:"title"`
	Authors     []string `json:"authors"`
	Tags        []string `json:"tags"`
	Series      string   `json:"series"`
	SeriesIndex float64  `json:"seriesIndex"`
	Language    string   `json:"language"`
	Description string   `json:"description"`
	Publisher   string   `json:"publisher"`
	Date        string   `json:"date"`
	Edited      bool     `json:"edited"`
}

// MetadataEditor edits book metadata in the index DB (never the file).
// It is the DB-only analogue of music's TagEditor.
type MetadataEditor struct {
	q           *db.Queries
	storageRoot string
}

// NewMetadataEditor creates a MetadataEditor backed by the given query set and
// storage root directory.
func NewMetadataEditor(q *db.Queries, storageRoot string) *MetadataEditor {
	return &MetadataEditor{q: q, storageRoot: storageRoot}
}

// resolve verifies node ownership and ebook-folder membership, then returns the
// book row. Returns ErrNotBook if no book is indexed for the node, or
// ErrNotInEbookFolder if the node is outside the configured ebook folder.
func (e *MetadataEditor) resolve(ctx context.Context, userID, nodeID string) (db.Book, error) {
	uid, err := db.ParseUUID(userID)
	if err != nil {
		return db.Book{}, err
	}
	nid, err := db.ParseUUID(nodeID)
	if err != nil {
		return db.Book{}, err
	}

	node, err := e.q.GetNodeForUser(ctx, db.GetNodeForUserParams{ID: nid, UserID: uid})
	if err != nil {
		return db.Book{}, err
	}

	if err := e.assertInEbookFolder(ctx, uid, node); err != nil {
		return db.Book{}, err
	}

	book, err := e.q.GetBookByNode(ctx, nid)
	if err != nil {
		return db.Book{}, ErrNotBook
	}
	return book, nil
}

// assertInEbookFolder checks that node's disk path is inside the user's
// configured ebook folder.
func (e *MetadataEditor) assertInEbookFolder(ctx context.Context, uid pgtype.UUID, node db.Node) error {
	es, err := e.q.GetEbookSettings(ctx, uid)
	if err != nil || !es.FolderNodeID.Valid {
		return ErrNotInEbookFolder
	}

	folder, err := e.q.GetNode(ctx, es.FolderNodeID)
	if err != nil || !folder.DiskPath.Valid {
		return ErrNotInEbookFolder
	}

	// The node is valid if it IS the folder (exact match) or is nested inside it.
	base := strings.TrimSuffix(folder.DiskPath.String, "/") + "/"
	if !node.DiskPath.Valid {
		return ErrNotInEbookFolder
	}
	if node.DiskPath.String != folder.DiskPath.String &&
		!strings.HasPrefix(node.DiskPath.String, base) {
		return ErrNotInEbookFolder
	}
	return nil
}

// Read returns the current metadata for the book at nodeID.
func (e *MetadataEditor) Read(ctx context.Context, userID, nodeID string) (BookMeta, error) {
	book, err := e.resolve(ctx, userID, nodeID)
	if err != nil {
		return BookMeta{}, err
	}

	// BookAuthors returns []BookAuthorsRow{Name, SortName}; map to []string.
	authorRows, err := e.q.BookAuthors(ctx, book.ID)
	if err != nil {
		return BookMeta{}, err
	}
	authors := make([]string, len(authorRows))
	for i, r := range authorRows {
		authors[i] = r.Name
	}

	// BookTags returns []string directly.
	tags, err := e.q.BookTags(ctx, book.ID)
	if err != nil {
		return BookMeta{}, err
	}

	m := BookMeta{
		Title:       book.Title,
		Authors:     authors,
		Tags:        tags,
		Series:      book.Series.String,
		Language:    book.Language.String,
		Description: book.Description.String,
		Publisher:   book.Publisher.String,
		Date:        book.PublishedDate.String,
		Edited:      book.MetadataEdited,
	}
	if book.SeriesIndex.Valid {
		m.SeriesIndex = float64(book.SeriesIndex.Float32)
	}
	return m, nil
}

// Write updates book metadata in the DB and sets metadata_edited=true.
// m.Edited is ignored; the flag is always set to true by the query.
func (e *MetadataEditor) Write(ctx context.Context, userID, nodeID string, m BookMeta) error {
	book, err := e.resolve(ctx, userID, nodeID)
	if err != nil {
		return err
	}

	title := strings.TrimSpace(m.Title)
	sortTitle := strings.ToLower(title)

	var seriesIdx pgtype.Float4
	if m.SeriesIndex != 0 {
		seriesIdx = pgtype.Float4{Float32: float32(m.SeriesIndex), Valid: true}
	}

	text := func(s string) pgtype.Text {
		s = strings.TrimSpace(s)
		return pgtype.Text{String: s, Valid: s != ""}
	}

	if err := e.q.UpdateBookMetadata(ctx, db.UpdateBookMetadataParams{
		ID:            book.ID,
		UserID:        book.UserID,
		Title:         title,
		SortTitle:     sortTitle,
		Language:      text(m.Language),
		Description:   text(m.Description),
		Publisher:     text(m.Publisher),
		PublishedDate: text(m.Date),
		Series:        text(m.Series),
		SeriesIndex:   seriesIdx,
	}); err != nil {
		return err
	}

	if err := e.q.ClearBookAuthors(ctx, book.ID); err != nil {
		return err
	}
	for _, a := range m.Authors {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if err := e.q.InsertBookAuthor(ctx, db.InsertBookAuthorParams{
			BookID:   book.ID,
			Name:     a,
			SortName: strings.ToLower(a),
		}); err != nil {
			return err
		}
	}

	if err := e.q.ClearBookTags(ctx, book.ID); err != nil {
		return err
	}
	for _, tg := range m.Tags {
		tg = strings.TrimSpace(tg)
		if tg == "" {
			continue
		}
		if err := e.q.InsertBookTag(ctx, db.InsertBookTagParams{
			BookID: book.ID,
			Tag:    tg,
		}); err != nil {
			return err
		}
	}
	return nil
}

// BulkInput is the set of fields a folder-bulk edit applies. A nil slice / nil
// pointer means "leave unchanged"; Authors/Tags apply only when non-empty; a
// scalar pointer applies only when the pointed-to string is non-empty (empty
// never clears).
type BulkInput struct {
	Authors   []string
	Tags      []string
	Series    *string
	Language  *string
	Publisher *string
	Date      *string
}

// BulkFailure records a per-book error that occurred during WriteFolder.
type BulkFailure struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

// BulkResult summarises the outcome of a WriteFolder call.
type BulkResult struct {
	Affected int           `json:"affected"`
	Updated  int           `json:"updated"`
	Failed   []BulkFailure `json:"failed"`
}

// folderBooks returns the indexed books under folderNodeID after verifying the
// folder is owned by the user and inside the configured ebook folder.
func (e *MetadataEditor) folderBooks(ctx context.Context, userID, folderNodeID string) ([]db.Book, error) {
	uid, err := db.ParseUUID(userID)
	if err != nil {
		return nil, err
	}
	fid, err := db.ParseUUID(folderNodeID)
	if err != nil {
		return nil, err
	}
	folder, err := e.q.GetNodeForUser(ctx, db.GetNodeForUserParams{ID: fid, UserID: uid})
	if err != nil {
		return nil, err
	}
	if !folder.IsDir {
		return nil, ErrNotBook
	}
	if err := e.assertInEbookFolder(ctx, uid, folder); err != nil {
		return nil, err
	}
	nodes, err := e.q.ListFileNodesUnderFolder(ctx, fid)
	if err != nil {
		return nil, err
	}
	var books []db.Book
	for _, n := range nodes {
		if !n.DiskPath.Valid {
			continue
		}
		b, err := e.q.GetBookByNode(ctx, n.ID)
		if err != nil {
			continue // not an indexed book — skip
		}
		books = append(books, b)
	}
	return books, nil
}

// CountFolderBooks returns the number of indexed books under folderNodeID.
func (e *MetadataEditor) CountFolderBooks(ctx context.Context, userID, folderNodeID string) (int, error) {
	books, err := e.folderBooks(ctx, userID, folderNodeID)
	if err != nil {
		return 0, err
	}
	return len(books), nil
}

// applyBulk overlays the requested bulk fields onto a book's current metadata.
// Only non-empty values are applied; empty values never clear existing data.
func applyBulk(cur BookMeta, in BulkInput) BookMeta {
	if len(in.Authors) > 0 {
		cur.Authors = in.Authors
	}
	if len(in.Tags) > 0 {
		cur.Tags = in.Tags
	}
	if in.Series != nil && *in.Series != "" {
		cur.Series = *in.Series
	}
	if in.Language != nil && *in.Language != "" {
		cur.Language = *in.Language
	}
	if in.Publisher != nil && *in.Publisher != "" {
		cur.Publisher = *in.Publisher
	}
	if in.Date != nil && *in.Date != "" {
		cur.Date = *in.Date
	}
	return cur
}

// WriteFolder applies in to every indexed book under folderNodeID. Per-book
// errors are collected in BulkResult.Failed; the loop always continues.
func (e *MetadataEditor) WriteFolder(ctx context.Context, userID, folderNodeID string, in BulkInput) (BulkResult, error) {
	books, err := e.folderBooks(ctx, userID, folderNodeID)
	if err != nil {
		return BulkResult{}, err
	}
	res := BulkResult{Affected: len(books)}
	for _, b := range books {
		nodeID := db.UUIDString(b.NodeID)
		cur, err := e.Read(ctx, userID, nodeID)
		if err != nil {
			res.Failed = append(res.Failed, BulkFailure{Path: db.UUIDString(b.ID), Error: err.Error()})
			continue
		}
		if err := e.Write(ctx, userID, nodeID, applyBulk(cur, in)); err != nil {
			res.Failed = append(res.Failed, BulkFailure{Path: db.UUIDString(b.ID), Error: err.Error()})
			continue
		}
		res.Updated++
	}
	return res, nil
}

// Reset clears the metadata_edited flag and re-indexes the book from its file,
// restoring the file's original metadata.
func (e *MetadataEditor) Reset(ctx context.Context, userID, nodeID string) error {
	book, err := e.resolve(ctx, userID, nodeID)
	if err != nil {
		return err
	}

	if err := e.q.SetBookMetadataEdited(ctx, db.SetBookMetadataEditedParams{
		ID:             book.ID,
		MetadataEdited: false,
	}); err != nil {
		return err
	}

	node, err := e.q.GetNode(ctx, book.NodeID)
	if err != nil || !node.DiskPath.Valid {
		return err
	}

	abs := filepath.Join(e.storageRoot, node.DiskPath.String)
	return NewIndexer(e.q, e.storageRoot).IndexNode(ctx, userID, nodeID, abs)
}
