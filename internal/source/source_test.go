package source

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/corygyarmathy/matrix-display/internal/render"
	"github.com/corygyarmathy/matrix-display/internal/sun"
)

// fake is a source that fails while down is set.
type fake struct {
	down    atomic.Bool
	fetches atomic.Int32
}

func (f *fake) Fetch(ctx context.Context) (Update, error) {
	f.fetches.Add(1)
	if f.down.Load() {
		return nil, errors.New("unreachable")
	}
	return nil, nil
}

// A source that is down for longer than its threshold lights the stale
// marker, and clears it once it recovers.
func TestStaleSourceShowsAndClears(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		const width, height = 16, 4
		r := render.Renderer{Width: width, Height: height, Location: sun.Greenwich}
		corner := func(store *Store) [3]uint16 {
			return r.Render(time.Now(), store.Snapshot()).At(width-1, height-1)
		}

		store := new(Store)
		src := new(fake)
		src.down.Store(true)
		go Run(ctx, store, Config{Name: "fake", Interval: 30 * time.Second, StaleAfter: 3 * time.Minute}, src)
		synctest.Wait()
		normal := corner(store)

		time.Sleep(2 * time.Minute)
		synctest.Wait()
		if got := corner(store); got != normal {
			t.Fatalf("down for 2 of 3 minutes: the corner is %v, want %v", got, normal)
		}

		time.Sleep(2 * time.Minute)
		synctest.Wait()
		if got := corner(store); got == normal {
			t.Fatal("down for 4 of 3 minutes: the corner shows no marker")
		}

		// The backoff is capped at the interval, so it fetches within 30s.
		src.down.Store(false)
		time.Sleep(30 * time.Second)
		synctest.Wait()
		if got := corner(store); got != normal {
			t.Fatalf("after recovering, the corner is %v, want %v", got, normal)
		}
	})
}

// Failed fetches back off, doubling up to MaxBackoff; successful ones wait
// Interval.
func TestRunBacksOff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		src := new(fake)
		src.down.Store(true)
		go Run(ctx, new(Store), Config{Name: "fake", Interval: time.Hour, MaxBackoff: 8 * time.Second}, src)

		// Fetches at 0, 1, 3, 7, 15, 23, 31 seconds.
		time.Sleep(31*time.Second + time.Millisecond)
		synctest.Wait()
		if got := src.fetches.Load(); got != 7 {
			t.Errorf("%d fetches in 31s, want 7", got)
		}

		src.down.Store(false)
		time.Sleep(8 * time.Second) // The 8th fetch succeeds.
		time.Sleep(59 * time.Minute)
		synctest.Wait()
		if got := src.fetches.Load(); got != 8 {
			t.Errorf("%d fetches, want 8 within the hour after a success", got)
		}
	})
}

// A fetch that hangs is cut off at Timeout and counts as a failure.
func TestRunTimesOutFetches(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		var calls atomic.Int32
		hang := sourceFunc(func(ctx context.Context) (Update, error) {
			calls.Add(1)
			<-ctx.Done()
			return nil, ctx.Err()
		})
		go Run(ctx, new(Store), Config{Name: "hang", Interval: time.Hour, Timeout: 5 * time.Second}, hang)
		time.Sleep(5*time.Second + time.Second + time.Millisecond) // Timeout, then a 1s backoff.
		synctest.Wait()
		if got := calls.Load(); got != 2 {
			t.Errorf("%d fetches, want 2", got)
		}
	})
}

func TestUpdatesReachSnapshots(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		store := new(Store)
		before := store.Snapshot()
		src := sourceFunc(func(context.Context) (Update, error) {
			return func(s *render.State) {}, nil
		})
		go Run(ctx, store, Config{Name: "ok", Interval: time.Minute}, src)
		synctest.Wait()
		if _, ok := store.Snapshot().Sources["ok"]; !ok {
			t.Error("the source's health is not in the snapshot")
		}
		if len(before.Sources) != 0 {
			t.Error("an earlier snapshot changed")
		}
	})
}

type sourceFunc func(context.Context) (Update, error)

func (f sourceFunc) Fetch(ctx context.Context) (Update, error) { return f(ctx) }
