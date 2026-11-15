package sleeper

import (
	"context"
	"sync"
	"time"
)

// PlayerCache holds Sleeper's full player dictionary in memory and refreshes
// it at most once per period. Sleeper's own guidance is to call /players/nfl
// no more than once a day, since the payload is large and rarely changes.
type PlayerCache struct {
	client *Client
	ttl    time.Duration

	mu        sync.RWMutex
	players   map[string]Player
	fetchedAt time.Time
}

func NewPlayerCache(client *Client, ttl time.Duration) *PlayerCache {
	return &PlayerCache{client: client, ttl: ttl}
}

// Get returns the cached player dictionary, refreshing it first if it is
// empty or older than the cache's TTL.
func (pc *PlayerCache) Get(ctx context.Context) (map[string]Player, error) {
	pc.mu.RLock()
	stale := pc.players == nil || time.Since(pc.fetchedAt) > pc.ttl
	players := pc.players
	pc.mu.RUnlock()

	if !stale {
		return players, nil
	}
	return pc.Refresh(ctx)
}

// Refresh fetches the player dictionary now, regardless of the cache's age,
// for reports that depend on up-to-the-minute data such as injury
// designations. If the fetch fails it falls back to the cached copy (when
// there is one) rather than failing a scheduled report over a transient
// error.
func (pc *PlayerCache) Refresh(ctx context.Context) (map[string]Player, error) {
	fresh, err := pc.client.GetAllPlayers(ctx)
	if err != nil {
		pc.mu.RLock()
		players := pc.players
		pc.mu.RUnlock()
		if players != nil {
			return players, nil
		}
		return nil, err
	}

	pc.mu.Lock()
	pc.players = fresh
	pc.fetchedAt = time.Now()
	pc.mu.Unlock()

	return fresh, nil
}
