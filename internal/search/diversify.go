package search

// DiversifyByBook reorders results to ensure at least minBooks different
// books appear in the top limit results. Uses round-robin across books
// ordered by their first appearance (highest-scoring first).
//
// If fewer than minBooks books exist in the result set, returns results as-is.
func DiversifyByBook(results []Result, minBooks, limit int) []Result {
	if len(results) <= 1 {
		return results
	}

	// Group results by book, preserving order of first appearance.
	type bookGroup struct {
		bookID  int64
		results []Result
	}
	var groups []bookGroup
	bookIndex := make(map[int64]int) // bookID -> index in groups

	for _, r := range results {
		bid := int64(0)
		if r.Book != nil {
			bid = r.Book.BookID
		}
		idx, exists := bookIndex[bid]
		if !exists {
			idx = len(groups)
			bookIndex[bid] = idx
			groups = append(groups, bookGroup{bookID: bid})
		}
		groups[idx].results = append(groups[idx].results, r)
	}

	// If we don't have enough books, just truncate and return.
	if len(groups) < minBooks {
		if len(results) > limit {
			return results[:limit]
		}
		return results
	}

	// Round-robin across book groups.
	var diversified []Result
	cursors := make([]int, len(groups))
	for len(diversified) < limit {
		added := false
		for gi := range groups {
			if cursors[gi] < len(groups[gi].results) {
				diversified = append(diversified, groups[gi].results[cursors[gi]])
				cursors[gi]++
				added = true
				if len(diversified) >= limit {
					break
				}
			}
		}
		if !added {
			break // all groups exhausted
		}
	}

	return diversified
}
