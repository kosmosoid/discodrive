package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"discodrive/internal/db"
	"discodrive/internal/music/tagwrite"
)

type mediaListingResp struct {
	Items []mediaItemDTO `json:"items"`
}

// addChild creates a child node of e.folder with real bytes on disk.
func (e *streamEnv) addChild(t *testing.T, name, mime string, isDir, isVault bool, content []byte) db.Node {
	t.Helper()
	rel := e.userID + "/media/" + name
	if isDir {
		if err := os.MkdirAll(filepath.Join(e.root, rel), 0o755); err != nil {
			t.Fatal(err)
		}
	} else if content != nil {
		if err := os.WriteFile(filepath.Join(e.root, rel), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var m pgtype.Text
	if mime != "" {
		m = pgtype.Text{String: mime, Valid: true}
	}
	node, err := e.q.CreateNode(e.ctx, db.CreateNodeParams{
		UserID: e.user.ID, ParentID: e.folder.ID, Name: name, IsDir: isDir,
		DiskPath: pgtype.Text{String: rel, Valid: true}, Mime: m, IsVault: isVault,
	})
	if err != nil {
		t.Fatalf("addChild %s: %v", name, err)
	}
	return node
}

// indexSong puts a node into the music index with artist and album names.
func (e *streamEnv) indexSong(t *testing.T, node db.Node, title, artist, album string, duration int32) {
	t.Helper()
	ar, err := e.q.UpsertArtist(e.ctx, db.UpsertArtistParams{UserID: e.user.ID, Name: artist, SortName: artist})
	if err != nil {
		t.Fatalf("artist: %v", err)
	}
	al, err := e.q.UpsertAlbum(e.ctx, db.UpsertAlbumParams{UserID: e.user.ID, ArtistID: ar.ID, Name: album})
	if err != nil {
		t.Fatalf("album: %v", err)
	}
	if _, err := e.q.UpsertSong(e.ctx, db.UpsertSongParams{
		UserID: e.user.ID, AlbumID: al.ID, ArtistID: ar.ID, NodeID: node.ID,
		Title: title, Duration: pgtype.Int4{Int32: duration, Valid: true},
		Track: pgtype.Int4{Int32: 7, Valid: true},
	}); err != nil {
		t.Fatalf("song: %v", err)
	}
}

func (e *streamEnv) mediaReq(t *testing.T, bearer, parentID, query string) *httptest.ResponseRecorder {
	t.Helper()
	h := e.svc.Middleware(http.HandlerFunc(e.s.handleMediaListing))
	rec := httptest.NewRecorder()
	req := authedReq(http.MethodGet, "/files/"+parentID+"/media"+query, bearer, "", parentID)
	h.ServeHTTP(rec, req)
	return rec
}

func (e *streamEnv) login(t *testing.T, email string) string {
	t.Helper()
	res, err := e.svc.Login(e.ctx, email, "password12")
	if err != nil {
		t.Fatalf("login %s: %v", email, err)
	}
	return res.Token
}

func TestMediaListingFiltersAndMetadata(t *testing.T) {
	e := buildStreamEnv(t, "media1@x.test")
	tok := e.login(t, "media1@x.test")

	// e.track (audio/mpeg) exists already; index it.
	e.indexSong(t, e.track, "Титул", "Артист", "Альбом", 215)
	flac := e.addChild(t, "raw.flac", "audio/flac", false, false, []byte("flacbytes"))
	e.addChild(t, "notes.txt", "text/plain", false, false, []byte("text"))
	e.addChild(t, "sub", "", true, false, nil)
	e.addChild(t, "secret.mp3", "audio/mpeg", false, true, []byte("vault"))

	rec := e.mediaReq(t, tok, db.UUIDString(e.folder.ID), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp mediaListingResp
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 playable items (txt, dir and vault excluded), got %d: %s", len(resp.Items), rec.Body.String())
	}
	byID := map[string]mediaItemDTO{}
	for _, it := range resp.Items {
		byID[it.NodeID] = it
	}
	indexed := byID[db.UUIDString(e.track.ID)]
	if !indexed.Indexed || indexed.Title != "Титул" || indexed.Artist != "Артист" || indexed.Album != "Альбом" {
		t.Fatalf("indexed item wrong: %+v", indexed)
	}
	if indexed.Duration == nil || *indexed.Duration != 215 || indexed.Track == nil || *indexed.Track != 7 {
		t.Fatalf("indexed duration/track wrong: %+v", indexed)
	}
	plain := byID[db.UUIDString(flac.ID)]
	if plain.Indexed || plain.Title != "" || plain.Mime != "audio/flac" {
		t.Fatalf("non-indexed item wrong: %+v", plain)
	}

	// The minted stream_url actually streams: parse token from it and hit the endpoint.
	u := indexed.StreamURL
	want := "/files/" + indexed.NodeID + "/stream?t="
	if len(u) <= len(want) || u[:len(want)] != want {
		t.Fatalf("stream_url shape: %q", u)
	}
	srec := httptest.NewRecorder()
	e.s.handleStream(srec, e.streamReq(indexed.NodeID, u[len(want):]))
	if srec.Code != http.StatusOK {
		t.Fatalf("stream via minted url: %d", srec.Code)
	}
}

func TestMediaListingSharedFolder(t *testing.T) {
	e := buildStreamEnv(t, "media2@x.test")
	if _, _, err := e.svc.Register(e.ctx, "viewer@x.test", "password12"); err != nil {
		t.Fatalf("register viewer: %v", err)
	}
	if _, err := e.files.ShareToUser(e.ctx, e.userID, db.UUIDString(e.folder.ID), "viewer@x.test", "read", nil); err != nil {
		t.Fatalf("share: %v", err)
	}
	viewerTok := e.login(t, "viewer@x.test")

	// Files inside the shared folder belong to the OWNER: a user_id filter would
	// return an empty list here — this test pins the ancestor-share semantics.
	rec := e.mediaReq(t, viewerTok, db.UUIDString(e.folder.ID), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp mediaListingResp
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Items) != 1 || resp.Items[0].NodeID != db.UUIDString(e.track.ID) {
		t.Fatalf("viewer must see the owner's track: %s", rec.Body.String())
	}
	// And the viewer's minted URL streams the owner's file.
	u := resp.Items[0].StreamURL
	prefix := "/files/" + resp.Items[0].NodeID + "/stream?t="
	srec := httptest.NewRecorder()
	e.s.handleStream(srec, e.streamReq(resp.Items[0].NodeID, u[len(prefix):]))
	if srec.Code != http.StatusOK {
		t.Fatalf("viewer stream: %d", srec.Code)
	}
}

func TestMediaListingForeignFolder(t *testing.T) {
	e := buildStreamEnv(t, "media3@x.test")
	if _, _, err := e.svc.Register(e.ctx, "stranger@x.test", "password12"); err != nil {
		t.Fatal(err)
	}
	rec := e.mediaReq(t, e.login(t, "stranger@x.test"), db.UUIDString(e.folder.ID), "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("stranger listing: expected 404, got %d", rec.Code)
	}
}

func TestMediaSingleNode(t *testing.T) {
	e := buildStreamEnv(t, "media4@x.test")
	tok := e.login(t, "media4@x.test")
	parent := db.UUIDString(e.folder.ID)

	// Non-indexed mp3 WITH real tags: the single-node variant reads them lazily.
	src, err := os.ReadFile(sampleMP3Path)
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	tagged := e.addChild(t, "tagged.mp3", "audio/mpeg", false, false, src)
	w, ok := tagwrite.For("mp3")
	if !ok {
		t.Fatal("no mp3 writer")
	}
	title, artist := "Лазурь", "Пишущий"
	if err := w.Apply(filepath.Join(e.root, e.userID+"/media/tagged.mp3"),
		tagwrite.Tags{Title: &title, Artist: &artist}, tagwrite.CoverKeep, nil); err != nil {
		t.Fatalf("apply tags: %v", err)
	}

	rec := e.mediaReq(t, tok, parent, "?node_id="+db.UUIDString(tagged.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	var item mediaItemDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	if item.Indexed {
		t.Fatalf("must not be indexed: %+v", item)
	}
	if item.Title != "Лазурь" || item.Artist != "Пишущий" {
		t.Fatalf("lazy tags not read: %+v", item)
	}
	if item.StreamURL == "" {
		t.Fatalf("no stream_url on single node")
	}

	// Parent mismatch → 404: the node must live in the folder named by the path.
	other := e.addChild(t, "elsewhere", "", true, false, nil)
	rec = e.mediaReq(t, tok, db.UUIDString(other.ID), "?node_id="+db.UUIDString(tagged.ID))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("parent mismatch: expected 404, got %d", rec.Code)
	}

	// Non-media node → 403.
	txt := e.addChild(t, "plain.txt", "text/plain", false, false, []byte("x"))
	rec = e.mediaReq(t, tok, parent, "?node_id="+db.UUIDString(txt.ID))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-media single: expected 403, got %d", rec.Code)
	}
}

// tinyPNG is a valid 1x1 PNG, enough for cover round-trips.
var tinyPNG = []byte{
	0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89,
	0x00, 0x00, 0x00, 0x0a, 'I', 'D', 'A', 'T',
	0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00, 0x05, 0x00, 0x01,
	0x0d, 0x0a, 0x2d, 0xb4,
	0x00, 0x00, 0x00, 0x00, 'I', 'E', 'N', 'D', 0xae, 0x42, 0x60, 0x82,
}

func (e *streamEnv) coverReq(t *testing.T, bearer, nodeID string) *httptest.ResponseRecorder {
	t.Helper()
	h := e.svc.Middleware(http.HandlerFunc(e.s.handleMediaCover))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodGet, "/files/"+nodeID+"/media-cover", bearer, "", nodeID))
	return rec
}

func TestMediaCover(t *testing.T) {
	e := buildStreamEnv(t, "cover1@x.test")
	tok := e.login(t, "cover1@x.test")

	// 1. No cover anywhere → 404. e.track is fake bytes, tags unreadable → no embedded.
	rec := e.coverReq(t, tok, db.UUIDString(e.track.ID))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("no cover: expected 404, got %d", rec.Code)
	}

	// 2. Sibling cover.jpg → served with image mime.
	if err := os.WriteFile(filepath.Join(e.root, e.userID+"/media/cover.jpg"), []byte("jpegbytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec = e.coverReq(t, tok, db.UUIDString(e.track.ID))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("sibling cover: code=%d ct=%q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec.Body.String() != "jpegbytes" {
		t.Fatalf("sibling cover body: %q", rec.Body.String())
	}

	// 3. Embedded cover wins over the sibling: mp3 fixture + tagwrite CoverReplace.
	// Note this file lives OUTSIDE any music folder — the point of this endpoint.
	src, err := os.ReadFile(sampleMP3Path)
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	embedded := e.addChild(t, "art.mp3", "audio/mpeg", false, false, src)
	w, _ := tagwrite.For("mp3")
	if err := w.Apply(filepath.Join(e.root, e.userID+"/media/art.mp3"),
		tagwrite.Tags{}, tagwrite.CoverReplace, &tagwrite.Cover{Data: tinyPNG, Mime: "image/png"}); err != nil {
		t.Fatalf("embed cover: %v", err)
	}
	rec = e.coverReq(t, tok, db.UUIDString(embedded.ID))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("embedded cover: code=%d ct=%q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec.Body.Len() != len(tinyPNG) {
		t.Fatalf("embedded cover bytes: got %d want %d", rec.Body.Len(), len(tinyPNG))
	}

	// 4. Access: a stranger gets 404, not the bytes.
	if _, _, err := e.svc.Register(e.ctx, "coverstranger@x.test", "password12"); err != nil {
		t.Fatal(err)
	}
	rec = e.coverReq(t, e.login(t, "coverstranger@x.test"), db.UUIDString(embedded.ID))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("stranger cover: expected 404, got %d", rec.Code)
	}
}
