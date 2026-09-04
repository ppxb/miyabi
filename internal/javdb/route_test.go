package javdb

import (
	"context"
	"errors"
	"reflect"
	"testing"
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

func TestSelectRouteReusesCachedHost(t *testing.T) {
	check := func(_ context.Context, host string) (time.Duration, map[string]any, error) {
		if host != "https://cached.example" {
			t.Fatalf("unexpected host %q", host)
		}
		return 12 * time.Millisecond, nil, nil
	}
	result, err := selectRoute(context.Background(), "https://cached.example/", check)
	if err != nil {
		t.Fatal(err)
	}
	if result.Host != "https://cached.example" || !result.ReusedCached {
		t.Fatalf("result = %#v", result)
	}
}

func TestSelectRouteChoosesFastestDynamicHost(t *testing.T) {
	check := func(_ context.Context, host string) (time.Duration, map[string]any, error) {
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

	result, err := selectRoute(context.Background(), "", check)
	if err != nil {
		t.Fatal(err)
	}
	if result.Host != bootstrapHosts[2] || result.Latency != 5*time.Millisecond {
		t.Fatalf("result = %#v", result)
	}
}

func TestSelectRouteFallsBackToFastestBootstrap(t *testing.T) {
	check := func(_ context.Context, host string) (time.Duration, map[string]any, error) {
		switch host {
		case bootstrapHosts[0]:
			return 30 * time.Millisecond, nil, nil
		case bootstrapHosts[1]:
			return 10 * time.Millisecond, nil, nil
		default:
			return 0, nil, errors.New("offline")
		}
	}

	result, err := selectRoute(context.Background(), "", check)
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
