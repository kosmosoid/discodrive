package subsonic

import "discodrive/internal/db"

// songFromSearchSongsRow extracts the db.Song fields embedded in a SearchSongsRow.
func songFromSearchSongsRow(r db.SearchSongsRow) db.Song {
	return db.Song{
		ID:            r.ID,
		UserID:        r.UserID,
		AlbumID:       r.AlbumID,
		ArtistID:      r.ArtistID,
		NodeID:        r.NodeID,
		Title:         r.Title,
		Track:         r.Track,
		Disc:          r.Disc,
		Duration:      r.Duration,
		Bitrate:       r.Bitrate,
		Suffix:        r.Suffix,
		ContentType:   r.ContentType,
		Size:          r.Size,
		Genre:         r.Genre,
		MusicbrainzID: r.MusicbrainzID,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
}

// searchSongRowToChild builds a Subsonic Child object from a SearchSongsRow.
// marks is used to populate starred/userRating for the song.
func searchSongRowToChild(r db.SearchSongsRow, marks userMarks) map[string]any {
	sUUID := db.UUIDString(r.ID)
	return buildSongChild(songFromSearchSongsRow(r), r.AlbumName, r.ArtistName, r.AlbumYear,
		marks.starredAt("song", sUUID), marks.ratingOf("song", sUUID))
}

// randomSongRowToChild builds a Subsonic Child object from a RandomAccessibleSongsRow.
// marks is used to populate starred/userRating for the song.
func randomSongRowToChild(r db.RandomAccessibleSongsRow, marks userMarks) map[string]any {
	s := db.Song{
		ID:            r.ID,
		UserID:        r.UserID,
		AlbumID:       r.AlbumID,
		ArtistID:      r.ArtistID,
		NodeID:        r.NodeID,
		Title:         r.Title,
		Track:         r.Track,
		Disc:          r.Disc,
		Duration:      r.Duration,
		Bitrate:       r.Bitrate,
		Suffix:        r.Suffix,
		ContentType:   r.ContentType,
		Size:          r.Size,
		Genre:         r.Genre,
		MusicbrainzID: r.MusicbrainzID,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
	sUUID := db.UUIDString(r.ID)
	return buildSongChild(s, r.AlbumName, r.ArtistName, r.AlbumYear,
		marks.starredAt("song", sUUID), marks.ratingOf("song", sUUID))
}

// similarRowToChild builds a Subsonic Child object from an AccessibleSongsByArtistRow.
func similarRowToChild(r db.AccessibleSongsByArtistRow, marks userMarks) map[string]any {
	s := db.Song{
		ID:            r.ID,
		UserID:        r.UserID,
		AlbumID:       r.AlbumID,
		ArtistID:      r.ArtistID,
		NodeID:        r.NodeID,
		Title:         r.Title,
		Track:         r.Track,
		Disc:          r.Disc,
		Duration:      r.Duration,
		Bitrate:       r.Bitrate,
		Suffix:        r.Suffix,
		ContentType:   r.ContentType,
		Size:          r.Size,
		Genre:         r.Genre,
		MusicbrainzID: r.MusicbrainzID,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
	sUUID := db.UUIDString(r.ID)
	return buildSongChild(s, r.AlbumName, r.ArtistName, r.AlbumYear,
		marks.starredAt("song", sUUID), marks.ratingOf("song", sUUID))
}

// similarGenreRowToChild builds a Subsonic Child object from a SimilarSongsByGenreRow.
func similarGenreRowToChild(r db.SimilarSongsByGenreRow, marks userMarks) map[string]any {
	s := db.Song{
		ID:            r.ID,
		UserID:        r.UserID,
		AlbumID:       r.AlbumID,
		ArtistID:      r.ArtistID,
		NodeID:        r.NodeID,
		Title:         r.Title,
		Track:         r.Track,
		Disc:          r.Disc,
		Duration:      r.Duration,
		Bitrate:       r.Bitrate,
		Suffix:        r.Suffix,
		ContentType:   r.ContentType,
		Size:          r.Size,
		Genre:         r.Genre,
		MusicbrainzID: r.MusicbrainzID,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
	sUUID := db.UUIDString(r.ID)
	return buildSongChild(s, r.AlbumName, r.ArtistName, r.AlbumYear,
		marks.starredAt("song", sUUID), marks.ratingOf("song", sUUID))
}

// topSongRowToChild builds a Subsonic Child object from a TopSongsByArtistNameRow.
func topSongRowToChild(r db.TopSongsByArtistNameRow, marks userMarks) map[string]any {
	s := db.Song{
		ID:            r.ID,
		UserID:        r.UserID,
		AlbumID:       r.AlbumID,
		ArtistID:      r.ArtistID,
		NodeID:        r.NodeID,
		Title:         r.Title,
		Track:         r.Track,
		Disc:          r.Disc,
		Duration:      r.Duration,
		Bitrate:       r.Bitrate,
		Suffix:        r.Suffix,
		ContentType:   r.ContentType,
		Size:          r.Size,
		Genre:         r.Genre,
		MusicbrainzID: r.MusicbrainzID,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
	sUUID := db.UUIDString(r.ID)
	return buildSongChild(s, r.AlbumName, r.ArtistName, r.AlbumYear,
		marks.starredAt("song", sUUID), marks.ratingOf("song", sUUID))
}

// genreSongRowToChild builds a Subsonic Child object from a SongsByGenreRow.
// marks is used to populate starred/userRating for the song.
func genreSongRowToChild(r db.SongsByGenreRow, marks userMarks) map[string]any {
	s := db.Song{
		ID:            r.ID,
		UserID:        r.UserID,
		AlbumID:       r.AlbumID,
		ArtistID:      r.ArtistID,
		NodeID:        r.NodeID,
		Title:         r.Title,
		Track:         r.Track,
		Disc:          r.Disc,
		Duration:      r.Duration,
		Bitrate:       r.Bitrate,
		Suffix:        r.Suffix,
		ContentType:   r.ContentType,
		Size:          r.Size,
		Genre:         r.Genre,
		MusicbrainzID: r.MusicbrainzID,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
	sUUID := db.UUIDString(r.ID)
	return buildSongChild(s, r.AlbumName, r.ArtistName, r.AlbumYear,
		marks.starredAt("song", sUUID), marks.ratingOf("song", sUUID))
}

// recentPlayRowToChild builds a Subsonic Child object from a RecentPlayHistoryRow.
// marks is used to populate starred/userRating for the song.
func recentPlayRowToChild(r db.RecentPlayHistoryRow, marks userMarks) map[string]any {
	s := db.Song{
		ID:            r.ID,
		UserID:        r.UserID,
		AlbumID:       r.AlbumID,
		ArtistID:      r.ArtistID,
		NodeID:        r.NodeID,
		Title:         r.Title,
		Track:         r.Track,
		Disc:          r.Disc,
		Duration:      r.Duration,
		Bitrate:       r.Bitrate,
		Suffix:        r.Suffix,
		ContentType:   r.ContentType,
		Size:          r.Size,
		Genre:         r.Genre,
		MusicbrainzID: r.MusicbrainzID,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
	sUUID := db.UUIDString(r.ID)
	return buildSongChild(s, r.AlbumName, r.ArtistName, r.AlbumYear,
		marks.starredAt("song", sUUID), marks.ratingOf("song", sUUID))
}
