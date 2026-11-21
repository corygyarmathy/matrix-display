// Package source runs the renderer's data sources (presence, alerts,
// calendar), each in its own goroutine, and publishes what they fetch into a
// Store the renderer reads a snapshot of on every frame.
package source

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/corygyarmathy/matrix-display/internal/render"
)

// Source fetches one kind of data.
type Source interface {
	// Fetch fetches the latest data, and returns an update that writes it
	// into the State, or nil if there is nothing to write. It must give up
	// when ctx ends.
	Fetch(ctx context.Context) (Update, error)
}

// Update writes a fetch's result into the State. It replaces the State's
// fields rather than changing what they hold (see render.State).
type Update func(*render.State)

// Config is how Run drives one source.
type Config struct {
	// Name identifies the source, in the State's health and in logs.
	Name string
	// Interval is the time between successful fetches.
	Interval time.Duration
	// Timeout bounds each fetch. Zero means 10 seconds.
	Timeout time.Duration
	// StaleAfter is how long the source can go without a successful fetch
	// before the display shows it is stale. Zero means 5 minutes.
	StaleAfter time.Duration
	// MaxBackoff caps the wait between failed fetches, which doubles from 1
	// second. Zero means a minute, or Interval if that is shorter.
	MaxBackoff time.Duration
}

func (c Config) withDefaults() Config {
	if c.Timeout == 0 {
		c.Timeout = 10 * time.Second
	}
	if c.StaleAfter == 0 {
		c.StaleAfter = 5 * time.Minute
	}
	if c.MaxBackoff == 0 {
		c.MaxBackoff = min(time.Minute, max(c.Interval, time.Second))
	}
	return c
}

// Store holds the State the sources publish into.
type Store struct {
	mu    sync.Mutex
	state render.State
}

// Snapshot returns the current State, which later updates don't change.
func (s *Store) Snapshot() render.State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.Clone()
}

func (s *Store) update(f func(*render.State)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(&s.state)
}

func (s *Store) setHealth(name string, h render.Health) {
	s.update(func(st *render.State) {
		if st.Sources == nil {
			st.Sources = make(map[string]render.Health)
		}
		st.Sources[name] = h
	})
}

// Run fetches from src into store until ctx ends: every Interval while it
// succeeds, and backing off while it fails. The source counts as fetched
// when Run starts, so it goes stale StaleAfter later if it never succeeds.
func Run(ctx context.Context, store *Store, cfg Config, src Source) {
	cfg = cfg.withDefaults()
	log := slog.With("source", cfg.Name)
	health := render.Health{LastOK: time.Now(), StaleAfter: cfg.StaleAfter}
	store.setHealth(cfg.Name, health)

	backoff := time.Second
	failing := false
	for {
		fetchCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		update, err := src.Fetch(fetchCtx)
		cancel()
		if ctx.Err() != nil {
			return
		}

		wait := cfg.Interval
		if err != nil {
			if !failing {
				log.Warn("fetch failed; retrying with backoff", "err", err)
			}
			failing = true
			wait = backoff
			backoff = min(2*backoff, cfg.MaxBackoff)
		} else {
			if failing {
				log.Info("fetching again")
			}
			failing = false
			backoff = time.Second
			health.LastOK = time.Now()
			if update != nil {
				store.update(update)
			}
			store.setHealth(cfg.Name, health)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}
