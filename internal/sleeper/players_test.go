package sleeper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestPlayerCacheRefreshBypassesTTL(t *testing.T) {
	var calls atomic.Int32
	status := []string{"", "Out"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n > 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte(`{"p1":{"player_id":"p1","injury_status":"` + status[n-1] + `"}}`))
	}))
	defer srv.Close()

	client := NewClient()
	client.baseURL = srv.URL
	cache := NewPlayerCache(client, 24*time.Hour)
	ctx := context.Background()

	if _, err := cache.Get(ctx); err != nil {
		t.Fatal(err)
	}
	// Within the TTL, Get serves the cached copy.
	if got, _ := cache.Get(ctx); got["p1"].InjuryStatus != "" || calls.Load() != 1 {
		t.Fatalf("Get within TTL refetched or changed data (calls=%d)", calls.Load())
	}

	got, err := cache.Refresh(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got["p1"].InjuryStatus != "Out" {
		t.Errorf("Refresh status = %q, want Out", got["p1"].InjuryStatus)
	}
	if got, _ := cache.Get(ctx); got["p1"].InjuryStatus != "Out" {
		t.Errorf("Get after Refresh = %q, want the refreshed Out", got["p1"].InjuryStatus)
	}

	// A failed refresh falls back to the cached copy.
	got, err = cache.Refresh(ctx)
	if err != nil || got["p1"].InjuryStatus != "Out" {
		t.Errorf("failed Refresh = %v, %v; want cached copy and no error", got["p1"], err)
	}
}
