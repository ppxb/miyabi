package javdb

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

var bootstrapHosts = []string{
	"https://jdforrepam.com",
	"https://apidd.spthgb.com",
	"https://apidd.czssdgz.com",
	"https://javdb.com",
}

const (
	backupKeyInput = "30820"
	backupIVInput  = "astarte"
	backupKeyConst = "WzE5OSwxNjksMTYwLDE3NCwxOTksMTA2LDEyNCwxNzQsMTM4LDE3MywxNjIsMTQ5LDE5MCwxNzksMTU3LDIwNiwxMjgsMjA5LDEyNSwxNzIsMTI4LDE4MiwxNjIsMTYxXQ=="
	backupIVConst  = "WzE1MSwxNDMsMTI3LDEwMywxOTksMTQwLDIwMCwxNjksMTU3LDE2MiwxNjUsMTAxLDE5OCwxNjMsMTc0LDE1NywyMDMsMTI1LDE1NiwxNjksMTQxLDIyMCwxMTEsMTYyXQ=="
)

var backupKey, backupIV = backupKeyMaterial()

type routeSelection struct {
	full  bool
	hosts []string
}

type startupData struct {
	// Decode optional backup data after probing so it cannot invalidate a healthy route.
	BackupDomainsData json.RawMessage `json:"backup_domains_data"`
}

type backupDomains struct {
	APIDomains []string `json:"apiDomains"`
}

// onStart records the request start after transport construction.
type probeFunc func(context.Context, string, func(time.Time)) (time.Duration, startupData, error)

type probeResult struct {
	host    string
	latency time.Duration
	startup startupData
	err     error
}

type probeEvent struct {
	probeResult
	started time.Time
}

type runningProbe struct {
	started  time.Time
	cancel   context.CancelFunc
	canceled bool
}

func selectRoute(ctx context.Context, options routeSelection, check probeFunc) (RouteStatus, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	events := make(chan probeEvent)
	running := make(map[string]*runningProbe)
	seen := make(map[string]bool)
	known := make(map[string]probeResult)

	start := func(host string) {
		if seen[host] {
			return
		}
		seen[host] = true
		probeCtx, stop := context.WithCancel(ctx)
		running[host] = &runningProbe{cancel: stop}
		go func() {
			latency, startup, err := check(probeCtx, host, func(started time.Time) {
				select {
				case events <- probeEvent{probeResult: probeResult{host: host}, started: started}:
				case <-ctx.Done():
				}
			})
			select {
			case events <- probeEvent{probeResult: probeResult{host: host, latency: latency, startup: startup, err: err}}:
			case <-ctx.Done():
			}
		}()
	}
	for _, host := range bootstrapHosts {
		start(host)
	}
	for _, host := range options.hosts {
		start(host)
	}

	var dynamic []string
	dynamicSeen := make(map[string]bool)
	var failures []error
	var best probeResult
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	for len(running) > 0 {
		var next time.Time
		// Until a dynamic source is found, a slow bootstrap may still be its
		// only source. Construction time never counts as request latency.
		if !options.full && len(dynamic) > 0 && best.host != "" {
			now := time.Now()
			for _, state := range running {
				if state.started.IsZero() || state.canceled {
					continue
				}
				// Preserve equal-latency results for the existing tie-break order.
				deadline := state.started.Add(best.latency + time.Nanosecond)
				if !now.Before(deadline) {
					state.cancel()
					state.canceled = true
				} else if next.IsZero() || deadline.Before(next) {
					next = deadline
				}
			}
		}
		var timeout <-chan time.Time
		if !next.IsZero() {
			if timer == nil {
				timer = time.NewTimer(time.Until(next))
			} else {
				timer.Reset(time.Until(next))
			}
			timeout = timer.C
		} else if timer != nil {
			timer.Stop()
		}
		select {
		case <-ctx.Done():
			return RouteStatus{}, ctx.Err()
		case event := <-events:
			state := running[event.host]
			if !event.started.IsZero() {
				state.started = event.started
				continue
			}
			state.cancel()
			delete(running, event.host)
			result := event.probeResult
			known[result.host] = result
			if result.err != nil {
				failures = append(failures, fmt.Errorf("%s: %w", result.host, result.err))
				continue
			}
			if best.host == "" || result.latency < best.latency {
				best = result
			}
			if options.full || len(dynamic) == 0 {
				hosts, err := apiHostsFromStartup(result.startup)
				if err != nil {
					failures = append(failures, fmt.Errorf("%s: %w", result.host, err))
				} else if len(hosts) > 0 {
					for _, host := range hosts {
						if !dynamicSeen[host] {
							dynamicSeen[host] = true
							dynamic = append(dynamic, host)
						}
						start(host)
					}
				}
			}
		case <-timeout:
		}
	}
	if err := ctx.Err(); err != nil {
		return RouteStatus{}, err
	}

	// Ties follow dynamic response order, then bootstrap and previously known hosts.
	hosts := append(dynamic, bootstrapHosts...)
	hosts = append(hosts, options.hosts...)
	result := RouteStatus{Candidates: routeCandidates(hosts, known)}
	var selected probeResult
	for _, host := range hosts {
		candidate := known[host]
		if candidate.err == nil && (selected.host == "" || candidate.latency < selected.latency) {
			selected = candidate
		}
	}
	if selected.host == "" {
		return result, fmt.Errorf("select JavDB route: %w", errors.Join(failures...))
	}
	result.Host = selected.host
	result.Latency = selected.latency
	return result, nil
}

func routeCandidates(hosts []string, known map[string]probeResult) []RouteCandidate {
	candidates := make([]RouteCandidate, 0, len(hosts))
	seen := make(map[string]bool, len(hosts))
	for _, host := range hosts {
		if seen[host] {
			continue
		}
		seen[host] = true
		candidate := RouteCandidate{Host: host, Status: RouteUntested}
		if result, ok := known[host]; ok {
			switch {
			case result.err == nil:
				candidate.Status = RouteAvailable
				candidate.Latency = result.latency
			case !errors.Is(result.err, context.Canceled):
				candidate.Status = RouteUnavailable
			}
		}
		candidates = append(candidates, candidate)
	}
	return candidates
}

func apiHostsFromStartup(startup startupData) ([]string, error) {
	if len(startup.BackupDomainsData) == 0 {
		return nil, nil
	}
	var encoded *string
	if err := json.Unmarshal(startup.BackupDomainsData, &encoded); err != nil {
		return nil, fmt.Errorf("decode startup backup_domains_data: %w", err)
	}
	if encoded == nil {
		return nil, errors.New("startup backup_domains_data is not a string")
	}
	payload, err := decryptBackupDomains(*encoded)
	if err != nil {
		return nil, err
	}

	hosts := make([]string, 0, len(payload.APIDomains))
	seen := make(map[string]bool, len(payload.APIDomains))
	for _, value := range payload.APIDomains {
		host, err := normalizeHost(value)
		if err != nil {
			return nil, err
		}
		if !seen[host] {
			seen[host] = true
			hosts = append(hosts, host)
		}
	}
	return hosts, nil
}

func decryptBackupDomains(encoded string) (backupDomains, error) {
	encrypted, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return backupDomains{}, fmt.Errorf("decode backup domains: %w", err)
	}
	if len(encrypted) == 0 || len(encrypted)%aes.BlockSize != 0 {
		return backupDomains{}, fmt.Errorf("decode backup domains: invalid cipher length %d", len(encrypted))
	}

	block, err := aes.NewCipher(backupKey)
	if err != nil {
		return backupDomains{}, fmt.Errorf("create backup domains cipher: %w", err)
	}
	plain := make([]byte, len(encrypted))
	cipher.NewCBCDecrypter(block, backupIV).CryptBlocks(plain, encrypted)
	plain, err = unpadPKCS7(plain)
	if err != nil {
		return backupDomains{}, fmt.Errorf("decode backup domains: %w", err)
	}
	if !utf8.Valid(plain) {
		return backupDomains{}, errors.New("decode backup domains: invalid UTF-8")
	}

	// An omitted list is optional; an explicit null list is malformed.
	payload := backupDomains{APIDomains: []string{}}
	if err := json.Unmarshal(plain, &payload); err != nil {
		return backupDomains{}, fmt.Errorf("decode backup domains JSON: %w", err)
	}
	if payload.APIDomains == nil {
		return backupDomains{}, errors.New("backup domains has no apiDomains")
	}
	return payload, nil
}

func backupKeyMaterial() ([]byte, []byte) {
	key, err := decryptConstant(backupKeyInput, backupKeyConst)
	if err != nil {
		panic(err)
	}
	iv, err := decryptConstant(backupIVInput, backupIVConst)
	if err != nil {
		panic(err)
	}
	return []byte(key), []byte(iv)
}

func decryptConstant(input, encoded string) (string, error) {
	sum := md5.Sum([]byte(input))
	key := hex.EncodeToString(sum[:])
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	var values []int
	if err := json.Unmarshal(raw, &values); err != nil {
		return "", err
	}
	decoded := make([]byte, len(values))
	for index, value := range values {
		decoded[index] = byte(value - int(key[min(index, len(key)-1)]))
	}
	result, err := base64.StdEncoding.DecodeString(string(decoded))
	if err != nil {
		return "", err
	}
	return string(result), nil
}

func unpadPKCS7(data []byte) ([]byte, error) {
	padding := int(data[len(data)-1])
	if padding == 0 || padding > aes.BlockSize || padding > len(data) {
		return nil, errors.New("invalid PKCS7 padding")
	}
	for _, value := range data[len(data)-padding:] {
		if int(value) != padding {
			return nil, errors.New("invalid PKCS7 padding")
		}
	}
	return data[:len(data)-padding], nil
}

func normalizeHost(value string) (string, error) {
	host := strings.TrimRight(strings.TrimSpace(value), "/")
	parsed, err := url.Parse(host)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return "", fmt.Errorf("invalid JavDB host %q", value)
	}
	if !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
		return "", fmt.Errorf("invalid JavDB host %q", value)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("invalid JavDB host %q", value)
	}
	return host, nil
}
