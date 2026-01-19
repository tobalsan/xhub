package db

import (
	"database/sql"
	"math"
	"sort"
)

// Search performs hybrid search combining BM25 (FTS5) and vector similarity
func (s *Store) Search(query string, limit int) ([]Bookmark, error) {
	if query == "" {
		return s.List(nil, limit)
	}

	// Get FTS results with BM25 scores
	// FTS5 can fail on special characters (spaces, quotes, operators)
	// Gracefully fall back to listing if FTS fails
	ftsResults, err := s.ftsSearch(query, 50)
	if err != nil {
		// Fall back to simple listing when FTS5 query fails
		ftsResults = nil
	}

	// Get vector results (if embeddings available)
	vecResults, err := s.vectorSearch(query, 50)
	if err != nil {
		// Vector search may fail if no embeddings, continue with FTS only
		vecResults = nil
	}

	// Combine results using reciprocal rank fusion
	combined := hybridRank(ftsResults, vecResults)

	// If no results from search, fall back to listing all bookmarks
	if len(combined) == 0 {
		return s.List(nil, limit)
	}

	// Limit results
	if len(combined) > limit {
		combined = combined[:limit]
	}

	// Fetch full bookmarks
	bookmarks := make([]Bookmark, 0, len(combined))
	for _, sr := range combined {
		b, err := s.Get(sr.ID)
		if err != nil {
			continue
		}
		bookmarks = append(bookmarks, *b)
	}

	return bookmarks, nil
}

type scoredResult struct {
	ID    string
	Score float64
	Rank  int
}

func (s *Store) ftsSearch(query string, limit int) ([]scoredResult, error) {
	// FTS5 search with BM25 ranking
	sqlQuery := `
		SELECT b.id, bm25(bookmarks_fts) as score
		FROM bookmarks_fts
		JOIN bookmarks b ON bookmarks_fts.rowid = b.rowid
		WHERE bookmarks_fts MATCH ?
		AND b.hidden = 0
		ORDER BY score
		LIMIT ?
	`

	rows, err := s.db.Query(sqlQuery, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []scoredResult
	rank := 1
	for rows.Next() {
		var id string
		var score float64
		if err := rows.Scan(&id, &score); err != nil {
			return nil, err
		}
		results = append(results, scoredResult{ID: id, Score: -score, Rank: rank}) // BM25 returns negative scores
		rank++
	}

	return results, rows.Err()
}

func (s *Store) vectorSearch(query string, limit int) ([]scoredResult, error) {
	// For now, we'll do a brute-force search over stored embeddings
	// This requires the query to be embedded first, which happens in the indexer
	// Here we return empty results - actual vector search will be done via embedding comparison
	return nil, nil
}

// SearchWithEmbedding performs vector search with a pre-computed query embedding
func (s *Store) SearchWithEmbedding(queryEmbedding []float32, limit int) ([]scoredResult, error) {
	embeddings, err := s.GetAllWithEmbeddings()
	if err != nil {
		return nil, err
	}

	var results []scoredResult
	for id, emb := range embeddings {
		if len(emb) == 0 {
			continue
		}
		score := cosineSimilarity(queryEmbedding, emb)
		results = append(results, scoredResult{ID: id, Score: score})
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Assign ranks
	for i := range results {
		results[i].Rank = i + 1
	}

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// HybridSearchWithEmbedding combines FTS and vector search
func (s *Store) HybridSearchWithEmbedding(query string, queryEmbedding []float32, limit int) ([]Bookmark, error) {
	if query == "" {
		return s.List(nil, limit)
	}

	// Get FTS results
	ftsResults, err := s.ftsSearch(query, 50)
	if err != nil && err != sql.ErrNoRows {
		// FTS might fail on special characters, continue without it
		ftsResults = nil
	}

	// Get vector results
	vecResults, err := s.SearchWithEmbedding(queryEmbedding, 50)
	if err != nil {
		vecResults = nil
	}

	// Combine results
	combined := hybridRank(ftsResults, vecResults)

	if len(combined) > limit {
		combined = combined[:limit]
	}

	// Fetch full bookmarks
	bookmarks := make([]Bookmark, 0, len(combined))
	for _, sr := range combined {
		b, err := s.Get(sr.ID)
		if err != nil {
			continue
		}
		bookmarks = append(bookmarks, *b)
	}

	return bookmarks, nil
}

// hybridRank combines results using Reciprocal Rank Fusion (RRF)
func hybridRank(ftsResults, vecResults []scoredResult) []scoredResult {
	const k = 60 // RRF constant

	scores := make(map[string]float64)

	for _, r := range ftsResults {
		scores[r.ID] += 1.0 / (float64(k) + float64(r.Rank))
	}

	for _, r := range vecResults {
		scores[r.ID] += 1.0 / (float64(k) + float64(r.Rank))
	}

	var results []scoredResult
	for id, score := range scores {
		results = append(results, scoredResult{ID: id, Score: score})
	}

	// Sort by combined score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}
