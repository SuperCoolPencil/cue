package search

import (
	"context"
	"strings"

	"github.com/SuperCoolPencil/cue/internal/domain"
)

// FilterItem represents a searchable item
type FilterItem struct {
	Item      domain.ListItem // *MediaItem or *Show
	Title     string
	Summary   string
	Type      domain.MediaType
	LibraryID string
}

// FilterResult represents a search result with match metadata
type FilterResult struct {
	FilterItem
	MatchedIndexes []int
	Score          int
}

// Service handles fuzzy search across libraries
type Service struct {
	store  domain.Store
	remote domain.SearchClient
}

// NewService creates a new search service
func NewService(store domain.Store) *Service {
	return &Service{
		store: store,
	}
}

// SetRemote enables server-side search fallback.
func (s *Service) SetRemote(remote domain.SearchClient) {
	s.remote = remote
}

// SearchRemote runs a server-side search when a backend is available.
func (s *Service) SearchRemote(ctx context.Context, query string) ([]FilterResult, error) {
	if s.remote == nil || strings.TrimSpace(query) == "" {
		return nil, nil
	}
	items, err := s.remote.Search(ctx, query)
	if err != nil {
		return nil, err
	}
	results := make([]FilterResult, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		results = append(results, FilterResult{
			FilterItem: FilterItem{
				Item:      item,
				Title:     item.Title,
				Summary:   item.Summary,
				Type:      item.Type,
				LibraryID: item.LibraryID,
			},
		})
	}
	return results, nil
}

// FilterLocal searches cached data directly
func (s *Service) FilterLocal(query string, libraries []domain.Library) []FilterResult {
	if query == "" {
		return nil
	}

	var items []FilterItem
	for _, lib := range libraries {
		items = append(items, s.gatherLibraryItems(lib)...)
	}

	if len(items) == 0 {
		return nil
	}

	targets := make([]SearchTarget, len(items))
	for i, item := range items {
		targets[i] = SearchTarget{
			Title:   item.Title,
			Summary: item.Summary,
		}
	}

	matches := FuzzySearchTargets(query, targets)

	results := make([]FilterResult, len(matches))
	for i, match := range matches {
		results[i] = FilterResult{
			FilterItem:     items[match.Index],
			MatchedIndexes: match.MatchedIndexes,
			Score:          match.Score,
		}
	}

	return results
}

func (s *Service) gatherLibraryItems(lib domain.Library) []FilterItem {
	var items []FilterItem

	switch lib.Type {
	case "movie":
		if movies, ok := s.store.GetMovies(lib.ID); ok {
			for _, m := range movies {
				items = append(items, FilterItem{
					Item:      m,
					Title:     m.Title,
					Summary:   m.Summary,
					Type:      domain.MediaTypeMovie,
					LibraryID: lib.ID,
				})
			}
		}
	case "show":
		if shows, ok := s.store.GetShows(lib.ID); ok {
			for _, sh := range shows {
				items = append(items, FilterItem{
					Item:      sh,
					Title:     sh.Title,
					Summary:   sh.Summary,
					Type:      domain.MediaTypeShow,
					LibraryID: lib.ID,
				})
			}
		}
	case "mixed":
		if content, ok := s.store.GetMixedContent(lib.ID); ok {
			for _, item := range content {
				switch v := item.(type) {
				case *domain.MediaItem:
					items = append(items, FilterItem{
						Item:      v,
						Title:     v.Title,
						Summary:   v.Summary,
						Type:      domain.MediaTypeMovie,
						LibraryID: lib.ID,
					})
				case *domain.Show:
					items = append(items, FilterItem{
						Item:      v,
						Title:     v.Title,
						Summary:   v.Summary,
						Type:      domain.MediaTypeShow,
						LibraryID: lib.ID,
					})
				}
			}
		}
	}

	return items
}
