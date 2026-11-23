package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/projector22/abp/internal/config"
)

// queueIDs renders a queue's episode ids in order, e.g. "a3,b2,a2".
func queueIDs(q *Queue) string {
	var s []string
	for _, it := range q.Items {
		s = append(s, it.EpisodeID)
	}
	return strings.Join(s, ",")
}

// TestBuildQueueOrder checks both playlist orderings against the fake server's
// episodes (a1=1000, b1=2000, a2=3000, b2=4000, a3=5000):
//   - the default and an explicit "oldest" queue oldest first and keep the oldest
//     `limit` episodes,
//   - "newest" queues newest first and keeps the newest `limit` episodes,
//   - finished episodes are skipped in both directions,
//   - Unfinished always reports the full count before truncation.
func TestBuildQueueOrder(t *testing.T) {
	pods := []config.Podcast{{ID: "podA"}, {ID: "podB"}}

	tests := []struct {
		name     string
		order    string
		limit    int
		finished map[string]bool
		want     string
		total    int
	}{
		{"default is oldest first", "", 25, nil, "a1,b1,a2,b2,a3", 5},
		{"explicit oldest", config.OrderOldest, 25, nil, "a1,b1,a2,b2,a3", 5},
		{"oldest keeps the oldest", config.OrderOldest, 3, nil, "a1,b1,a2", 5},
		{"newest first", config.OrderNewest, 25, nil, "a3,b2,a2,b1,a1", 5},
		{"newest keeps the newest", config.OrderNewest, 3, nil, "a3,b2,a2", 5},
		{"newest skips finished", config.OrderNewest, 25, map[string]bool{"podA/a3": true}, "b2,a2,b1,a1", 4},
		{"oldest skips finished", config.OrderOldest, 25, map[string]bool{"podA/a1": true}, "b1,a2,b2,a3", 4},
		{"order is case-insensitive", "Newest", 2, nil, "a3,b2", 5},
		{"unknown order falls back to oldest", "sideways", 2, nil, "a1,b1", 5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fin := tc.finished
			if fin == nil {
				fin = map[string]bool{}
			}
			f := &fakeABS{finished: fin}
			c, _ := newEnv(t, f, f.handler())
			pl := config.Playlist{ID: "x", Limit: tc.limit, Order: tc.order, Podcasts: pods}
			q, err := BuildQueue(context.Background(), c, pl)
			if err != nil {
				t.Fatal(err)
			}
			if got := queueIDs(q); got != tc.want {
				t.Errorf("order %q limit %d: got %s, want %s", tc.order, tc.limit, got, tc.want)
			}
			if q.Unfinished != tc.total {
				t.Errorf("Unfinished = %d, want %d", q.Unfinished, tc.total)
			}
		})
	}
}

// TestRunNewestFirstReordersExistingPlaylist checks that switching a playlist
// from oldest to newest is applied to the same server playlist in place (no
// delete/recreate), and that the server ends up holding the newest-first order.
func TestRunNewestFirstReordersExistingPlaylist(t *testing.T) {
	f := &fakeABS{finished: map[string]bool{}}
	c, st := newEnv(t, f, f.handler())
	ctx := context.Background()

	pl := config.Playlist{
		ID: "x", Name: "X", Library: "lib", Limit: 25,
		Podcasts: []config.Podcast{{ID: "podA"}, {ID: "podB"}},
	}
	if _, err := Run(ctx, c, pl, st, false); err != nil {
		t.Fatal(err)
	}
	if got := ids(f.pl.sorted()); got != "a1,b1,a2,b2,a3" {
		t.Fatalf("oldest-first playlist = %s", got)
	}

	pl.Order = config.OrderNewest
	if _, err := Run(ctx, c, pl, st, false); err != nil {
		t.Fatal(err)
	}
	if got := ids(f.pl.sorted()); got != "a3,b2,a2,b1,a1" {
		t.Errorf("newest-first playlist = %s", got)
	}
	if f.creates != 1 {
		t.Errorf("playlist was recreated: %d creates, want 1", f.creates)
	}
}
