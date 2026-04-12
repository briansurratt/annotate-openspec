package index

import "strings"

// computeBacklinks performs a second pass over all loaded entries to compute
// BacklinkCount for each entry. An entry's count is the number of other entries
// whose Links list contains the entry's Path or any of its Aliases.
//
// This must be called after all entries are loaded so that the full link graph
// is available.
func (idx *Index) computeBacklinks() {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Reset all counts before recomputing.
	for _, e := range idx.entries {
		e.BacklinkCount = 0
	}

	// Build a target→entry map: for each entry, register its path and each
	// alias as lookup keys pointing to that entry.
	targetToEntry := make(map[string]*NoteEntry, len(idx.entries)*2)
	for _, e := range idx.entries {
		targetToEntry[e.Path] = e
		// Also index by title for [[Title]] style links.
		if e.Title != "" {
			targetToEntry[e.Title] = e
		}
		for _, alias := range e.Aliases {
			targetToEntry[alias] = e
		}
	}

	// For each entry, check each outgoing link and increment the target's count.
	for _, src := range idx.entries {
		for _, link := range src.Links {
			// Match by exact path or by title/alias.
			if target, ok := targetToEntry[link]; ok && target.Path != src.Path {
				target.BacklinkCount++
				continue
			}
			// Also try basename match (links often omit the directory prefix).
			for _, target := range idx.entries {
				if target.Path == src.Path {
					continue
				}
				base := strings.TrimSuffix(target.Path[strings.LastIndex(target.Path, "/")+1:], ".md")
				if strings.EqualFold(link, base) || strings.EqualFold(link, target.Title) {
					target.BacklinkCount++
					break
				}
			}
		}
	}
}
