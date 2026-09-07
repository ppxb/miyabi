package javdb

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"
	"reflect"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

const encryptedBackupDomains = "JCxJQTR1DerICeuy4lmmWJuj2sRqgbDdvL2Nru5I6BmGb+GmAKKAUbjeLL1r+rFe" +
	"Oxq+Kb3g2MOSXYpvd9dA7Pds+G6brFTtRy7EQ0s4DkIaUfAzoKgMWldPRI/0IvUj" +
	"OvVkn1t0/nUIEz2LTWmcKx5sj3BVtIV5XEiRtS8fUGvVSddw6Fy7g9nJ/iN5OxFC" +
	"ypbRPK0dd6+09Vx3ALU/9kI39VeBlNZE7/Vjnr2nc0MZg3PIZHCt9dlldO9uS7GM" +
	"LU+LHXFq29VbyGGkXxlOuO+dE4ejYK1CJ9Qx14FuR1xWx3p8rOHo1INDE7LmqgZy" +
	"/3vDlRY8hHbdDr81tKWBAS/PXcOakVZGNuEiOf6OKtQR9J3M44MUStw+k5AZ9jh0" +
	"KhblvYeTdA79l1b+byubUqyDLP5XiEkyT2yQ8JTB/wHfH6Otg5/5NoI22nODaQjK" +
	"UaFDDnzr0S2Vwbp0uu68GAov458mHuuIUleBSI4TGqA="

func TestAPIHostsFromStartup(t *testing.T) {
	hosts, err := apiHostsFromStartup(map[string]any{"backup_domains_data": encryptedBackupDomains})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://apidd.spthgb.com", "https://apidd.czssdgz.com"}
	if !reflect.DeepEqual(hosts, want) {
		t.Fatalf("hosts = %v, want %v", hosts, want)
	}
}

func TestSelectRouteFullRetainsAvailableManualPreference(t *testing.T) {
	check := func(_ context.Context, host string, _ func(time.Time)) (time.Duration, map[string]any, error) {
		if host == "https://cached.example" {
			return 12 * time.Millisecond, nil, nil
		}
		return time.Millisecond, nil, nil
	}
	result, err := selectRoute(t.Context(), routeSelection{full: true, preferredHost: "https://cached.example"}, check)
	if err != nil {
		t.Fatal(err)
	}
	if result.Host != "https://cached.example" || !result.Manual || len(result.Candidates) != len(bootstrapHosts)+1 {
		t.Fatalf("result = %#v", result)
	}
	for _, candidate := range result.Candidates {
		if candidate.Status != RouteAvailable {
			t.Fatalf("candidate was not measured: %+v", candidate)
		}
	}
}

func TestSelectRouteChoosesFastestDynamicHost(t *testing.T) {
	check := func(_ context.Context, host string, _ func(time.Time)) (time.Duration, map[string]any, error) {
		switch host {
		case bootstrapHosts[0]:
			return 20 * time.Millisecond, map[string]any{"backup_domains_data": encryptedBackupDomains}, nil
		case bootstrapHosts[1]:
			return 30 * time.Millisecond, nil, nil
		case bootstrapHosts[2]:
			return 5 * time.Millisecond, nil, nil
		case bootstrapHosts[3]:
			return 0, nil, errors.New("offline")
		default:
			return 0, nil, errors.New("unexpected host")
		}
	}

	result, err := selectRoute(context.Background(), routeSelection{}, check)
	if err != nil {
		t.Fatal(err)
	}
	if result.Host != bootstrapHosts[2] || result.Latency != 5*time.Millisecond {
		t.Fatalf("result = %#v", result)
	}
}

func TestSelectRouteFallsBackToFastestBootstrap(t *testing.T) {
	check := func(_ context.Context, host string, _ func(time.Time)) (time.Duration, map[string]any, error) {
		switch host {
		case bootstrapHosts[0]:
			return 30 * time.Millisecond, nil, nil
		case bootstrapHosts[1]:
			return 10 * time.Millisecond, nil, nil
		default:
			return 0, nil, errors.New("offline")
		}
	}

	result, err := selectRoute(context.Background(), routeSelection{}, check)
	if err != nil {
		t.Fatal(err)
	}
	if result.Host != bootstrapHosts[1] || result.Latency != 10*time.Millisecond {
		t.Fatalf("result = %#v", result)
	}
}

func TestAPIHostsFromStartupAllowsMissingDynamicData(t *testing.T) {
	hosts, err := apiHostsFromStartup(map[string]any{})
	if err != nil || len(hosts) != 0 {
		t.Fatalf("hosts = %v, error = %v", hosts, err)
	}
}

func TestSelectRouteStartsDynamicProbesBeforeSlowBootstrapsFinish(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const dynamicHost = "https://dynamic.example"
		startup := startupWithDynamicHost(t, dynamicHost)
		check := func(ctx context.Context, host string, onStart func(time.Time)) (time.Duration, map[string]any, error) {
			onStart(time.Now())
			switch host {
			case bootstrapHosts[0]:
				time.Sleep(10 * time.Millisecond)
				return 10 * time.Millisecond, startup, nil
			case dynamicHost:
				time.Sleep(2 * time.Millisecond)
				return 2 * time.Millisecond, nil, nil
			default:
				<-ctx.Done()
				return 0, nil, ctx.Err()
			}
		}
		started := time.Now()
		result, err := selectRoute(t.Context(), routeSelection{}, check)
		if err != nil || result.Host != dynamicHost || time.Since(started) > 20*time.Millisecond {
			t.Fatalf("selection = %+v, error = %v, duration = %v", result, err, time.Since(started))
		}
	})
}

func TestSelectRouteDoesNotCountTransportConstructionAsLatency(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		check := func(ctx context.Context, host string, onStart func(time.Time)) (time.Duration, map[string]any, error) {
			if host == bootstrapHosts[2] {
				time.Sleep(60 * time.Millisecond)
			}
			onStart(time.Now())
			switch host {
			case bootstrapHosts[0]:
				time.Sleep(20 * time.Millisecond)
				return 20 * time.Millisecond, map[string]any{"backup_domains_data": encryptedBackupDomains}, nil
			case bootstrapHosts[2]:
				time.Sleep(5 * time.Millisecond)
				return 5 * time.Millisecond, nil, nil
			default:
				<-ctx.Done()
				return 0, nil, ctx.Err()
			}
		}
		result, err := selectRoute(t.Context(), routeSelection{}, check)
		if err != nil || result.Host != bootstrapHosts[2] || result.Latency != 5*time.Millisecond {
			t.Fatalf("selection = %+v, error = %v", result, err)
		}
	})
}

func TestSelectRouteWaitsForDynamicSourceEvenWhenBootstrapIsSlow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const dynamicHost = "https://dynamic.example"
		startup := startupWithDynamicHost(t, dynamicHost)
		check := func(ctx context.Context, host string, onStart func(time.Time)) (time.Duration, map[string]any, error) {
			onStart(time.Now())
			switch host {
			case bootstrapHosts[0]:
				time.Sleep(5 * time.Millisecond)
				return 5 * time.Millisecond, nil, nil
			case bootstrapHosts[1]:
				select {
				case <-time.After(50 * time.Millisecond):
					return 50 * time.Millisecond, startup, nil
				case <-ctx.Done():
					return 0, nil, ctx.Err()
				}
			case dynamicHost:
				time.Sleep(time.Millisecond)
				return time.Millisecond, nil, nil
			default:
				return 0, nil, errors.New("offline")
			}
		}
		result, err := selectRoute(t.Context(), routeSelection{}, check)
		if err != nil || result.Host != dynamicHost {
			t.Fatalf("selection = %+v, error = %v", result, err)
		}
	})
}

func TestSelectRouteFullMeasuresSlowKnownAndNewDynamicCandidates(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const firstDynamic = "https://first.example"
		const secondDynamic = "https://second.example"
		const previousHost = "https://previous.example"
		firstStartup := startupWithDynamicHost(t, firstDynamic)
		secondStartup := startupWithDynamicHost(t, secondDynamic)
		var mu sync.Mutex
		calls := make(map[string]int)
		check := func(ctx context.Context, host string, onStart func(time.Time)) (time.Duration, map[string]any, error) {
			mu.Lock()
			calls[host]++
			mu.Unlock()
			onStart(time.Now())
			delay := 80 * time.Millisecond
			var startup map[string]any
			switch host {
			case bootstrapHosts[0]:
				delay, startup = 10*time.Millisecond, firstStartup
			case bootstrapHosts[1]:
				delay, startup = 40*time.Millisecond, secondStartup
			case firstDynamic:
				delay = time.Millisecond
			case secondDynamic:
				delay = 2 * time.Millisecond
			}
			select {
			case <-time.After(delay):
				if host == bootstrapHosts[3] {
					return 0, nil, context.DeadlineExceeded
				}
				return delay, startup, nil
			case <-ctx.Done():
				return 0, nil, ctx.Err()
			}
		}
		started := time.Now()
		result, err := selectRoute(t.Context(), routeSelection{
			full: true, hosts: []string{previousHost, bootstrapHosts[0], previousHost},
		}, check)
		if err != nil || result.Host != firstDynamic || time.Since(started) < 80*time.Millisecond {
			t.Fatalf("result = %+v, error = %v, elapsed = %v", result, err, time.Since(started))
		}
		if len(result.Candidates) != len(bootstrapHosts)+3 {
			t.Fatalf("missing candidates: %+v", result.Candidates)
		}
		for _, candidate := range result.Candidates {
			want := RouteAvailable
			if candidate.Host == bootstrapHosts[3] {
				want = RouteUnavailable
			}
			if candidate.Status != want || calls[candidate.Host] != 1 {
				t.Fatalf("candidate = %+v, calls = %d", candidate, calls[candidate.Host])
			}
		}
	})
}

func TestSelectRouteFullReturnsResultsWhenAllCandidatesFail(t *testing.T) {
	check := func(context.Context, string, func(time.Time)) (time.Duration, map[string]any, error) {
		return 0, nil, context.DeadlineExceeded
	}
	result, err := selectRoute(t.Context(), routeSelection{full: true}, check)
	if err == nil || len(result.Candidates) != len(bootstrapHosts) {
		t.Fatalf("result = %+v, error = %v", result, err)
	}
	for _, candidate := range result.Candidates {
		if candidate.Status != RouteUnavailable {
			t.Fatalf("candidate = %+v", candidate)
		}
	}
}

func startupWithDynamicHost(t *testing.T, host string) map[string]any {
	t.Helper()
	plain := []byte(`{"apiDomains":["` + host + `"]}`)
	padding := aes.BlockSize - len(plain)%aes.BlockSize
	for range padding {
		plain = append(plain, byte(padding))
	}
	block, err := aes.NewCipher(backupKey)
	if err != nil {
		t.Fatal(err)
	}
	encrypted := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, backupIV).CryptBlocks(encrypted, plain)
	return map[string]any{"backup_domains_data": base64.StdEncoding.EncodeToString(encrypted)}
}
