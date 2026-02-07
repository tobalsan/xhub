package db

import (
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"math"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	db *sql.DB
}

func NewStore(dataDir string) (*Store, error) {
	dbPath := filepath.Join(dataDir, "xhub.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	if err != nil {
		return nil, err
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS bookmarks (
		id TEXT PRIMARY KEY,
		source TEXT NOT NULL,
		url TEXT NOT NULL UNIQUE,
		title TEXT,
		summary TEXT,
		keywords TEXT,
		notes TEXT,
		raw_content TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		scraped_at TIMESTAMP,
		scrape_status TEXT DEFAULT 'pending',
		hidden INTEGER DEFAULT 0
	);

	CREATE INDEX IF NOT EXISTS idx_bookmarks_source ON bookmarks(source);
	CREATE INDEX IF NOT EXISTS idx_bookmarks_scrape_status ON bookmarks(scrape_status);

	CREATE TABLE IF NOT EXISTS bookmarks_vec (
		id TEXT PRIMARY KEY,
		embedding BLOB
	);

	CREATE TABLE IF NOT EXISTS metadata (
		key TEXT PRIMARY KEY,
		value TEXT
	);
	`

	_, err := s.db.Exec(schema)
	if err != nil {
		return err
	}

	// Check if FTS table needs to be rebuilt (add url column)
	return s.migrateFTS()
}

func (s *Store) migrateFTS() error {
	// Check if bookmarks_fts table exists and has url column
	var tableName string
	err := s.db.QueryRow(`
		SELECT name FROM sqlite_master 
		WHERE type='table' AND name='bookmarks_fts'
	`).Scan(&tableName)
	if err != nil {
		// FTS table doesn't exist, create it fresh
		return s.createFTSTable()
	}

	// Check if url column exists
	var colName string
	err = s.db.QueryRow(`
		SELECT name FROM pragma_table_info('bookmarks_fts') 
		WHERE name='url'
	`).Scan(&colName)
	if err != nil {
		// url column doesn't exist, need to rebuild FTS table
		return s.rebuildFTSTable()
	}

	return nil
}

func (s *Store) createFTSTable() error {
	schema := `
	CREATE VIRTUAL TABLE IF NOT EXISTS bookmarks_fts USING fts5(
		title, summary, keywords, notes, url,
		content='bookmarks',
		content_rowid='rowid'
	);

	CREATE TRIGGER IF NOT EXISTS bookmarks_ai AFTER INSERT ON bookmarks BEGIN
		INSERT INTO bookmarks_fts(rowid, title, summary, keywords, notes, url)
		VALUES (new.rowid, new.title, new.summary, new.keywords, new.notes, new.url);
	END;

	CREATE TRIGGER IF NOT EXISTS bookmarks_ad AFTER DELETE ON bookmarks BEGIN
		INSERT INTO bookmarks_fts(bookmarks_fts, rowid, title, summary, keywords, notes, url)
		VALUES ('delete', old.rowid, old.title, old.summary, old.keywords, old.notes, old.url);
	END;

	CREATE TRIGGER IF NOT EXISTS bookmarks_au AFTER UPDATE ON bookmarks BEGIN
		INSERT INTO bookmarks_fts(bookmarks_fts, rowid, title, summary, keywords, notes, url)
		VALUES ('delete', old.rowid, old.title, old.summary, old.keywords, old.notes, old.url);
		INSERT INTO bookmarks_fts(rowid, title, summary, keywords, notes, url)
		VALUES (new.rowid, new.title, new.summary, new.keywords, new.notes, new.url);
	END;
	`

	_, err := s.db.Exec(schema)
	if err != nil {
		return err
	}

	// Populate FTS table with existing data
	_, err = s.db.Exec(`
		INSERT INTO bookmarks_fts(rowid, title, summary, keywords, notes, url)
		SELECT rowid, title, summary, keywords, notes, url FROM bookmarks
	`)
	return err
}

func (s *Store) rebuildFTSTable() error {
	// Drop old triggers
	_, err := s.db.Exec(`DROP TRIGGER IF EXISTS bookmarks_ai`)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`DROP TRIGGER IF EXISTS bookmarks_ad`)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`DROP TRIGGER IF EXISTS bookmarks_au`)
	if err != nil {
		return err
	}

	// Drop old FTS table
	_, err = s.db.Exec(`DROP TABLE IF EXISTS bookmarks_fts`)
	if err != nil {
		return err
	}

	// Create new FTS table with url column
	return s.createFTSTable()
}

func generateID(url string) string {
	hash := sha256.Sum256([]byte(url))
	return hex.EncodeToString(hash[:8])
}

func (s *Store) Upsert(b *Bookmark) error {
	_, err := s.UpsertReturningNew(b)
	return err
}

// UpsertReturningNew inserts or updates a bookmark and returns true if it was a new insert.
func (s *Store) UpsertReturningNew(b *Bookmark) (bool, error) {
	if b.ID == "" {
		b.ID = generateID(b.URL)
	}
	now := time.Now()
	b.UpdatedAt = now
	if b.CreatedAt.IsZero() {
		b.CreatedAt = now
	}

	// Check if URL already exists
	var existingID string
	err := s.db.QueryRow(`SELECT id FROM bookmarks WHERE url = ?`, b.URL).Scan(&existingID)
	isNew := err == sql.ErrNoRows

	query := `
	INSERT INTO bookmarks (id, source, url, title, summary, keywords, notes, raw_content, created_at, updated_at, scraped_at, scrape_status, hidden)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(url) DO UPDATE SET
		title = COALESCE(excluded.title, bookmarks.title),
		summary = COALESCE(excluded.summary, bookmarks.summary),
		keywords = COALESCE(excluded.keywords, bookmarks.keywords),
		notes = COALESCE(excluded.notes, bookmarks.notes),
		raw_content = COALESCE(excluded.raw_content, bookmarks.raw_content),
		updated_at = excluded.updated_at,
		scraped_at = COALESCE(excluded.scraped_at, bookmarks.scraped_at),
		scrape_status = COALESCE(excluded.scrape_status, bookmarks.scrape_status)
	`

	var scrapedAt interface{}
	if !b.ScrapedAt.IsZero() {
		scrapedAt = b.ScrapedAt
	}

	_, err = s.db.Exec(query,
		b.ID, b.Source, b.URL, b.Title, b.Summary, b.Keywords, b.Notes, b.RawContent,
		b.CreatedAt, b.UpdatedAt, scrapedAt, b.ScrapeStatus, b.Hidden,
	)
	return isNew, err
}

func (s *Store) Get(id string) (*Bookmark, error) {
	query := `SELECT id, source, url, title, summary, keywords, notes, raw_content, created_at, updated_at, scraped_at, scrape_status, hidden FROM bookmarks WHERE id = ?`

	var b Bookmark
	var scrapedAt sql.NullTime
	err := s.db.QueryRow(query, id).Scan(
		&b.ID, &b.Source, &b.URL, &b.Title, &b.Summary, &b.Keywords, &b.Notes, &b.RawContent,
		&b.CreatedAt, &b.UpdatedAt, &scrapedAt, &b.ScrapeStatus, &b.Hidden,
	)
	if err != nil {
		return nil, err
	}
	if scrapedAt.Valid {
		b.ScrapedAt = scrapedAt.Time
	}
	return &b, nil
}

func (s *Store) GetByURL(url string) (*Bookmark, error) {
	query := `SELECT id, source, url, title, summary, keywords, notes, raw_content, created_at, updated_at, scraped_at, scrape_status, hidden FROM bookmarks WHERE url = ?`

	var b Bookmark
	var scrapedAt sql.NullTime
	err := s.db.QueryRow(query, url).Scan(
		&b.ID, &b.Source, &b.URL, &b.Title, &b.Summary, &b.Keywords, &b.Notes, &b.RawContent,
		&b.CreatedAt, &b.UpdatedAt, &scrapedAt, &b.ScrapeStatus, &b.Hidden,
	)
	if err != nil {
		return nil, err
	}
	if scrapedAt.Valid {
		b.ScrapedAt = scrapedAt.Time
	}
	return &b, nil
}

func (s *Store) Delete(id string) error {
	// Delete embedding first
	_, _ = s.db.Exec(`DELETE FROM bookmarks_vec WHERE id = ?`, id)
	// Delete bookmark
	_, err := s.db.Exec(`DELETE FROM bookmarks WHERE id = ?`, id)
	return err
}

func (s *Store) List(sources []string, limit int) ([]Bookmark, error) {
	query := `SELECT id, source, url, title, summary, keywords, notes, created_at, updated_at, scrape_status, hidden FROM bookmarks WHERE hidden = 0`

	var args []interface{}
	if len(sources) > 0 {
		query += ` AND source IN (`
		for i, src := range sources {
			if i > 0 {
				query += ","
			}
			query += "?"
			args = append(args, src)
		}
		query += `)`
	}

	query += ` ORDER BY CASE WHEN source IN ('raindrop', 'github', 'x') THEN created_at ELSE updated_at END DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bookmarks []Bookmark
	for rows.Next() {
		var b Bookmark
		if err := rows.Scan(&b.ID, &b.Source, &b.URL, &b.Title, &b.Summary, &b.Keywords, &b.Notes, &b.CreatedAt, &b.UpdatedAt, &b.ScrapeStatus, &b.Hidden); err != nil {
			return nil, err
		}
		bookmarks = append(bookmarks, b)
	}
	return bookmarks, rows.Err()
}

func (s *Store) GetPending(limit int) ([]Bookmark, error) {
	query := `SELECT id, source, url, title, summary, keywords, notes, raw_content, created_at, updated_at, scraped_at, scrape_status, hidden FROM bookmarks WHERE scrape_status = 'pending' OR scrape_status = 'failed' LIMIT ?`

	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bookmarks []Bookmark
	for rows.Next() {
		var b Bookmark
		var scrapedAt sql.NullTime
		if err := rows.Scan(&b.ID, &b.Source, &b.URL, &b.Title, &b.Summary, &b.Keywords, &b.Notes, &b.RawContent, &b.CreatedAt, &b.UpdatedAt, &scrapedAt, &b.ScrapeStatus, &b.Hidden); err != nil {
			return nil, err
		}
		if scrapedAt.Valid {
			b.ScrapedAt = scrapedAt.Time
		}
		bookmarks = append(bookmarks, b)
	}
	return bookmarks, rows.Err()
}

func (s *Store) UpdateEmbedding(id string, embedding []float32) error {
	blob := float32SliceToBytes(embedding)
	_, err := s.db.Exec(`INSERT OR REPLACE INTO bookmarks_vec (id, embedding) VALUES (?, ?)`, id, blob)
	return err
}

func float32SliceToBytes(s []float32) []byte {
	b := make([]byte, len(s)*4)
	for i, v := range s {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(v))
	}
	return b
}

func bytesToFloat32Slice(b []byte) []float32 {
	if len(b)%4 != 0 {
		return nil
	}
	s := make([]float32, len(b)/4)
	for i := range s {
		s[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return s
}

func (s *Store) GetMetadata(key string) (string, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM metadata WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func (s *Store) SetMetadata(key, value string) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO metadata (key, value) VALUES (?, ?)`, key, value)
	return err
}

func (s *Store) Count() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM bookmarks WHERE hidden = 0`).Scan(&count)
	return count, err
}

func (s *Store) Update(b *Bookmark) error {
	b.UpdatedAt = time.Now()

	query := `UPDATE bookmarks SET title = ?, summary = ?, keywords = ?, notes = ?, raw_content = ?, updated_at = ?, scraped_at = ?, scrape_status = ?, hidden = ? WHERE id = ?`

	var scrapedAt interface{}
	if !b.ScrapedAt.IsZero() {
		scrapedAt = b.ScrapedAt
	}

	_, err := s.db.Exec(query, b.Title, b.Summary, b.Keywords, b.Notes, b.RawContent, b.UpdatedAt, scrapedAt, b.ScrapeStatus, b.Hidden, b.ID)
	return err
}

func (s *Store) GetAllWithEmbeddings() (map[string][]float32, error) {
	rows, err := s.db.Query(`SELECT id, embedding FROM bookmarks_vec`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]float32)
	for rows.Next() {
		var id string
		var blob []byte
		if err := rows.Scan(&id, &blob); err != nil {
			return nil, err
		}
		result[id] = bytesToFloat32Slice(blob)
	}
	return result, rows.Err()
}

func (s *Store) DB() *sql.DB {
	return s.db
}

// GetOrphanedBySource returns bookmarks from a source whose URLs are not in the given set.
// Used to detect items that were removed from the source.
func (s *Store) GetOrphanedBySource(source string, currentURLs []string) ([]Bookmark, error) {
	if len(currentURLs) == 0 {
		// If no URLs provided, all items from this source are orphaned
		return s.getBookmarksBySource(source)
	}

	// Build URL set for exclusion
	query := `SELECT id, source, url, title FROM bookmarks WHERE source = ? AND url NOT IN (`
	args := []interface{}{source}
	for i, url := range currentURLs {
		if i > 0 {
			query += ","
		}
		query += "?"
		args = append(args, url)
	}
	query += `)`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orphans []Bookmark
	for rows.Next() {
		var b Bookmark
		if err := rows.Scan(&b.ID, &b.Source, &b.URL, &b.Title); err != nil {
			return nil, err
		}
		orphans = append(orphans, b)
	}
	return orphans, rows.Err()
}

// getBookmarksBySource returns all bookmarks from a given source.
func (s *Store) getBookmarksBySource(source string) ([]Bookmark, error) {
	query := `SELECT id, source, url, title FROM bookmarks WHERE source = ?`
	rows, err := s.db.Query(query, source)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bookmarks []Bookmark
	for rows.Next() {
		var b Bookmark
		if err := rows.Scan(&b.ID, &b.Source, &b.URL, &b.Title); err != nil {
			return nil, err
		}
		bookmarks = append(bookmarks, b)
	}
	return bookmarks, rows.Err()
}

// MarkForReprocess resets items to pending so they get re-scraped/re-summarized/re-embedded.
// Clears raw_content, summary, keywords to force full reprocessing.
func (s *Store) MarkForReprocess(ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	query := `UPDATE bookmarks SET scrape_status = 'pending', raw_content = '', summary = '', keywords = '' WHERE id IN (`
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		if i > 0 {
			query += ","
		}
		query += "?"
		args[i] = id
	}
	query += `)`

	_, err := s.db.Exec(query, args...)
	return err
}
