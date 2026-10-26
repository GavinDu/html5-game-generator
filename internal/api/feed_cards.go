package api

import (
	"context"
	"net/url"
	"strconv"
	"time"

	"doodlequest/internal/model"
)

const FeedCardSchemaVersion = 1

type feedCreator struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
}

type feedPreview struct {
	PosterURL     string  `json:"poster_url,omitempty"`
	ClipURL       string  `json:"clip_url,omitempty"`
	Aspect        float64 `json:"aspect,omitempty"`
	DurationMS    int     `json:"duration_ms,omitempty"`
	DominantColor string  `json:"dominant_color,omitempty"`
	Status        string  `json:"status"`
}

type feedCardStats struct {
	Likes       int `json:"likes,omitempty"`
	PlayStarts  int `json:"play_starts,omitempty"`
	Completions int `json:"completions,omitempty"`
	Replays     int `json:"replays,omitempty"`
	Shares      int `json:"shares,omitempty"`
}

type feedCardViewer struct {
	Liked     bool `json:"liked"`
	Saved     bool `json:"saved"`
	Following bool `json:"following"`
}

type feedCardPackage struct {
	URL               string `json:"url"`
	MinRuntimeVersion string `json:"min_runtime_version"`
}

type feedCard struct {
	GameID  string          `json:"game_id"`
	Version string          `json:"version,omitempty"`
	Title   string          `json:"title"`
	Caption string          `json:"caption,omitempty"`
	Topics  []string        `json:"topics,omitempty"`
	Creator feedCreator     `json:"creator"`
	Preview feedPreview     `json:"preview"`
	Stats   *feedCardStats  `json:"stats,omitempty"`
	Viewer  feedCardViewer  `json:"viewer"`
	Package feedCardPackage `json:"package"`
}

// buildFeedCards creates the additive, versioned lightweight card view without
// changing the legacy games payload or feed ranking/cursor behavior.
func (s *Server) buildFeedCards(ctx context.Context, games []*model.Game, viewer string, followed map[string]bool) ([]feedCard, error) {
	saved := map[string]bool{}
	liked := map[string]bool{}
	likes := map[string]int{}
	eventStats := map[string]feedCardStats{}
	profiles := map[string]creatorProfile{}

	s.socialMu.RLock()
	for id, profile := range s.profiles {
		profiles[id] = profile
	}
	for gameID, users := range s.reactions {
		likes[gameID] = len(users)
		if reaction, ok := users[viewer]; ok && reaction != "" {
			liked[gameID] = true
		}
	}
	for gameID, isSaved := range s.saves[viewer] {
		if isSaved {
			saved[gameID] = true
		}
	}
	events := append([]socialEvent(nil), s.events...)
	s.socialMu.RUnlock()

	if s.socialGraph != nil {
		dbCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		if viewer != "" {
			ids, err := s.socialGraph.ListSavedGames(dbCtx, viewer)
			if err != nil {
				cancel()
				return nil, err
			}
			saved = make(map[string]bool, len(ids))
			for _, id := range ids {
				saved[id] = true
			}
		}
		gameIDs := make([]string, 0, len(games))
		for _, game := range games {
			if game != nil {
				gameIDs = append(gameIDs, game.ID)
			}
		}
		counts, viewerReactions, err := s.socialGraph.ListReactionSummary(dbCtx, viewer, gameIDs)
		cancel()
		if err != nil {
			return nil, err
		}
		likes = counts
		liked = map[string]bool{}
		for gameID, reaction := range viewerReactions {
			if reaction != "" {
				liked[gameID] = true
			}
		}
	}

	for _, event := range events {
		stats := eventStats[event.GameID]
		switch event.Kind {
		case "play_start":
			stats.PlayStarts++
		case "completion":
			stats.Completions++
		case "replay":
			stats.Replays++
		case "share":
			stats.Shares++
		}
		eventStats[event.GameID] = stats
	}

	cards := make([]feedCard, 0, len(games))
	for _, game := range games {
		if game == nil {
			continue
		}
		version := ""
		if currentVersion := publishedGameVersion(game); currentVersion > 0 {
			version = strconv.Itoa(currentVersion)
		}
		stats := eventStats[game.ID]
		stats.Likes = likes[game.ID]
		var statsPtr *feedCardStats
		if stats.Likes > 0 || stats.PlayStarts > 0 || stats.Completions > 0 || stats.Replays > 0 || stats.Shares > 0 {
			statsPtr = &stats
		}
		preview := feedPreview{Status: "pending"}
		if game.Preview != nil && game.Preview.Version == publishedGameVersion(game) {
			preview = feedPreview{
				PosterURL: game.Preview.PosterURL, ClipURL: game.Preview.ClipURL,
				Aspect: game.Preview.Aspect, DurationMS: game.Preview.DurationMS,
				DominantColor: game.Preview.DominantColor, Status: game.Preview.Status,
			}
		}
		cards = append(cards, feedCard{
			GameID:  game.ID,
			Version: version,
			Title:   game.Title,
			Caption: game.Caption,
			Topics:  append([]string(nil), game.Topics...),
			Creator: feedCreator{ID: game.OwnerID, DisplayName: profiles[game.OwnerID].DisplayName, AvatarURL: profiles[game.OwnerID].AvatarURL},
			// Preview metadata comes only from a matching published
			// version's stored preview; otherwise the card stays
			// pending rather than inventing a URL or metrics.
			Preview: preview,
			Stats:   statsPtr,
			Viewer: feedCardViewer{
				Liked:     liked[game.ID],
				Saved:     saved[game.ID],
				Following: followed[game.OwnerID],
			},
			Package: feedCardPackage{
				URL:               "/v1/games/" + url.PathEscape(game.ID) + "/package",
				MinRuntimeVersion: MinRuntimeVersion,
			},
		})
	}
	return cards, nil
}
