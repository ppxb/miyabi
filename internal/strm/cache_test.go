package strm

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/pan"
)

func signedURL(id int, expires time.Time) string {
	return fmt.Sprintf("https://cdn.example/%d.mp4?sign=a%%2Bb&t=%d", id, expires.Unix())
}

func TestStreamURLCachesByFileAndUserAgent(t *testing.T) {
	relay, client := relayFixture(t, "pick-101")
	relay.database.File.Create().SetFileID("102").SetPickCode("pick-102").SetName("second.mp4").SetSize(1).ExecX(t.Context())
	var queries, calls int
	expires := time.Now().Add(time.Hour)
	relay.database.File.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			queries++
			return next.Query(ctx, query)
		})
	}))
	client.downloadURL = func(context.Context, string, string, string) (string, error) {
		calls++
		return signedURL(calls, expires), nil
	}
	for _, tc := range []struct {
		file, ua string
		wantCall int
	}{
		{"101", "", 1}, {"101", "  ", 1},
		{"101", "Infuse/7.5", 2}, {"101", " Infuse/7.5 ", 2},
		{"101", "VidHub/1.0", 3}, {"102", "Infuse/7.5", 4},
	} {
		address, err := relay.StreamURL(t.Context(), tc.file, tc.ua)
		if err != nil || address != signedURL(tc.wantCall, expires) {
			t.Fatalf("file=%s ua=%q address=%q err=%v", tc.file, tc.ua, address, err)
		}
		if calls != tc.wantCall || queries != tc.wantCall {
			t.Fatalf("calls=%d queries=%d, want %d each", calls, queries, tc.wantCall)
		}
	}
}

func TestStreamURLRefreshesBeforeSignatureExpires(t *testing.T) {
	for _, lifetime := range []time.Duration{time.Hour, 45 * time.Second} {
		t.Run(lifetime.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				relay, client := relayFixture(t, "pick-101")
				var calls int
				client.downloadURL = func(context.Context, string, string, string) (string, error) {
					calls++
					return signedURL(calls, time.Now().Add(lifetime)), nil
				}
				first, err := relay.StreamURL(t.Context(), "101", "")
				if err != nil {
					t.Fatal(err)
				}
				deadline := min(streamCacheTTL, lifetime-streamExpiryMargin)
				time.Sleep(deadline - time.Nanosecond)
				if address, err := relay.StreamURL(t.Context(), "101", ""); err != nil || address != first || calls != 1 {
					t.Fatalf("early refresh: calls=%d err=%v", calls, err)
				}
				time.Sleep(time.Nanosecond)
				if address, err := relay.StreamURL(t.Context(), "101", ""); err != nil || address == first || calls != 2 {
					t.Fatalf("missing refresh: calls=%d err=%v", calls, err)
				}
			})
		})
	}
}

func TestStreamCacheSkipsUncertainOrExpiringSignatures(t *testing.T) {
	now := time.Unix(1700000000, 0)
	for _, address := range []string{
		"https://cdn.example/video.mp4",
		"https://cdn.example/video.m3u8?auth_key=opaque",
		"https://cdn.example/video.mp4?t=invalid",
		"https://cdn.example/video.mp4?t=",
		"https://cdn.example/video.mp4?t=-1",
		"https://cdn.example/video.mp4?t=1700003600&expires=invalid",
		"https://cdn.example/video.mp4?t=1700003600&bad=%zz",
		signedURL(1, now), signedURL(1, now.Add(streamExpiryMargin)),
	} {
		relay := &Relay{}
		relay.remember("file", address, now)
		if cached := relay.cached("file", now); cached != "" {
			t.Errorf("unsafe signature cached: %s", address)
		}
	}
	address := "https://cdn.example/video.mp4?t=1700003600&expires=1700000045"
	if expires := streamCacheExpiry(address, now); !expires.Equal(now.Add(15 * time.Second)) {
		t.Errorf("earliest signature deadline = %v", expires)
	}
}

func TestStreamURLCachesFallbackOnlyWithKnownExpiry(t *testing.T) {
	for _, known := range []bool{true, false} {
		t.Run(fmt.Sprint(known), func(t *testing.T) {
			relay, client := relayFixture(t, "pick-101")
			address := "https://cdn.example/video.m3u8?auth_key=opaque"
			if known {
				address = signedURL(1, time.Now().Add(time.Hour))
			}
			var downloads, plays int
			client.downloadURL = func(context.Context, string, string, string) (string, error) {
				downloads++
				return "", pan.ErrDownloadUnavailable
			}
			client.playURL = func(context.Context, string, string, string) ([]pan.PlaySource, error) {
				plays++
				return []pan.PlaySource{{URL: address, Height: 1080}}, nil
			}
			for range 2 {
				if got, err := relay.StreamURL(t.Context(), "101", ""); err != nil || got != address {
					t.Fatalf("fallback address=%q err=%v", got, err)
				}
			}
			want := 2
			if known {
				want = 1
			}
			if downloads != want || plays != want {
				t.Fatalf("downloads=%d plays=%d want=%d each", downloads, plays, want)
			}
		})
	}
}

func TestStreamCacheBoundsAndReclaimsEntries(t *testing.T) {
	relay := &Relay{}
	now := time.Unix(1700000000, 0)
	for i := range streamCacheLimit {
		relay.remember(fmt.Sprint(i), signedURL(i, now.Add(time.Hour)), now.Add(time.Duration(i)*time.Millisecond))
	}
	relay.remember("new", signedURL(999, now.Add(time.Hour)), now.Add(time.Second))
	if len(relay.urls) != streamCacheLimit || relay.cached("0", now.Add(time.Second)) != "" || relay.cached("new", now.Add(time.Second)) == "" {
		t.Fatal("cache did not evict the earliest deadline at capacity")
	}
	if got := relay.cached("missing", now.Add(2*streamCacheTTL)); got != "" || len(relay.urls) != 0 {
		t.Fatal("access did not reclaim expired entries")
	}
	relay.remember("old", signedURL(1, now.Add(time.Hour)), now)
	relay.remember("new", signedURL(2, now.Add(time.Hour)), now.Add(2*streamCacheTTL))
	if len(relay.urls) != 1 {
		t.Fatal("write did not reclaim expired entries")
	}
}

func TestConcurrentStreamRequestsShareSuccessAndFailure(t *testing.T) {
	for _, failure := range []error{nil, errors.New("upstream unavailable")} {
		t.Run(fmt.Sprint(failure), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				relay, client := relayFixture(t, "pick-101")
				hold := make(chan struct{})
				var calls atomic.Int32
				address := signedURL(1, time.Now().Add(time.Hour))
				client.downloadURL = func(context.Context, string, string, string) (string, error) {
					calls.Add(1)
					<-hold
					return address, failure
				}
				const waiters = 12
				finished := make(chan error, waiters)
				for range waiters {
					go func() {
						got, err := relay.StreamURL(t.Context(), "101", "")
						if err == nil && got != address {
							t.Errorf("unexpected shared address %q", got)
						}
						finished <- err
					}()
				}
				synctest.Wait()
				close(hold)
				for range waiters {
					if err := <-finished; !errors.Is(err, failure) {
						t.Fatalf("shared error=%v, want %v", err, failure)
					}
				}
				if calls.Load() != 1 {
					t.Fatalf("concurrent requests made %d upstream calls", calls.Load())
				}
				if _, err := relay.StreamURL(t.Context(), "101", ""); !errors.Is(err, failure) {
					t.Fatal(err)
				}
				want := int32(1)
				if failure != nil {
					want = 2
				}
				if calls.Load() != want {
					t.Fatalf("subsequent request calls=%d, want %d", calls.Load(), want)
				}
			})
		})
	}
}

func TestStreamWaiterCancellationDoesNotCancelSharedLookup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		relay, client := relayFixture(t, "pick-101")
		hold := make(chan struct{})
		var upstream context.Context
		var calls int
		client.downloadURL = func(ctx context.Context, _, _, _ string) (string, error) {
			upstream = ctx
			calls++
			<-hold
			return signedURL(1, time.Now().Add(time.Hour)), ctx.Err()
		}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		first, second := make(chan error, 1), make(chan error, 1)
		go func() { _, err := relay.StreamURL(ctx, "101", ""); first <- err }()
		synctest.Wait()
		go func() { _, err := relay.StreamURL(t.Context(), "101", ""); second <- err }()
		synctest.Wait()
		cancel()
		if err := <-first; !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled waiter error=%v", err)
		}
		if upstream.Err() != nil {
			t.Fatal("waiter canceled the shared request")
		}
		close(hold)
		if err := <-second; err != nil || calls != 1 {
			t.Fatalf("remaining waiter error=%v calls=%d", err, calls)
		}
	})
}

func TestSharedStreamLookupHasBoundedLifetime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		relay, client := relayFixture(t, "pick-101")
		client.downloadURL = func(ctx context.Context, _, _, _ string) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		}
		start := time.Now()
		if _, err := relay.StreamURL(t.Context(), "101", ""); !errors.Is(err, context.DeadlineExceeded) || time.Since(start) != 45*time.Second {
			t.Fatalf("elapsed=%v error=%v", time.Since(start), err)
		}
	})
}

func TestCloseWaitsForStreamLookupAfterCallerLeaves(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		relay, client := relayFixture(t, "pick-101")
		hold := make(chan struct{})
		client.downloadURL = func(ctx context.Context, _, _, _ string) (string, error) {
			<-hold
			return signedURL(1, time.Now().Add(time.Hour)), ctx.Err()
		}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		finished := make(chan error, 1)
		go func() { _, err := relay.StreamURL(ctx, "101", ""); finished <- err }()
		synctest.Wait()
		cancel()
		if err := <-finished; !errors.Is(err, context.Canceled) {
			t.Fatalf("departed caller error=%v", err)
		}
		closed := make(chan struct{})
		go func() { relay.drive.Close(); close(closed) }()
		synctest.Wait()
		select {
		case <-closed:
			t.Error("drive closed while the shared lookup still used it")
		default:
		}
		if _, err := relay.StreamURL(t.Context(), "101", ""); !errors.Is(err, context.Canceled) {
			t.Errorf("closing drive accepted a new lookup: %v", err)
		}
		close(hold)
		<-closed
	})
}

func TestStreamCacheFollowsAuthorizationAndMountChanges(t *testing.T) {
	relay, client := relayFixture(t, "pick-101")
	var calls int
	expires := time.Now().Add(time.Hour)
	client.downloadURL = func(context.Context, string, string, string) (string, error) {
		calls++
		return signedURL(calls, expires), nil
	}
	check := func(want int) {
		t.Helper()
		for range 2 {
			address, err := relay.StreamURL(t.Context(), "101", "")
			if err != nil || calls != want || address != signedURL(want, expires) {
				t.Fatalf("calls=%d want=%d address=%q err=%v", calls, want, address, err)
			}
		}
	}
	login := func() {
		t.Helper()
		pending, err := relay.drive.BeginLogin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if status, err := relay.drive.LoginStatus(t.Context(), pending.ID); err != nil || status.State != pan.LoginAuthorized {
			t.Fatalf("login status=%v error=%v", status, err)
		}
	}
	check(1)
	login() // Reauthorizing the same account invalidates its old signed URLs.
	check(2)
	if _, err := relay.drive.SelectDirectory(t.Context(), "20"); err != nil {
		t.Fatal(err)
	}
	check(3)
	if _, err := relay.drive.SelectDirectory(t.Context(), "10"); err != nil {
		t.Fatal(err)
	}
	check(4)
	if _, err := relay.drive.Disconnect(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := relay.StreamURL(t.Context(), "101", ""); err == nil || calls != 4 {
		t.Fatalf("disconnected account reused cache: calls=%d err=%v", calls, err)
	}
	client.account = func(context.Context, string) (pan.Account, error) {
		return pan.Account{ID: "200"}, nil
	}
	login()
	if _, err := relay.drive.SelectDirectory(t.Context(), "10"); err != nil {
		t.Fatal(err)
	}
	check(5)
}

func TestStreamLookupRejectsSourceChangeDuringRequest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		relay, client := relayFixture(t, "pick-101")
		hold := make(chan struct{})
		var calls atomic.Int32
		client.downloadURL = func(context.Context, string, string, string) (string, error) {
			call := calls.Add(1)
			if call == 1 {
				<-hold
			}
			return signedURL(int(call), time.Now().Add(time.Hour)), nil
		}
		finished := make(chan error, 1)
		go func() { _, err := relay.StreamURL(t.Context(), "101", ""); finished <- err }()
		synctest.Wait()
		if _, err := relay.drive.SelectDirectory(t.Context(), "20"); err != nil {
			t.Fatal(err)
		}
		if _, err := relay.StreamURL(t.Context(), "101", ""); err != nil || calls.Load() != 2 {
			t.Fatalf("new source joined old lookup: calls=%d err=%v", calls.Load(), err)
		}
		close(hold)
		if err := <-finished; !errors.Is(err, drive.ErrSourceChanged) {
			t.Fatalf("old source returned a URL: %v", err)
		}
		address, err := relay.StreamURL(t.Context(), "101", "")
		if err != nil || address != signedURL(2, time.Now().Add(time.Hour)) || calls.Load() != 2 {
			t.Fatalf("old lookup poisoned the new cache: calls=%d address=%q err=%v", calls.Load(), address, err)
		}
	})
}
