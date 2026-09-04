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

type routeResult struct {
	Host         string
	Latency      time.Duration
	ReusedCached bool
}

type probe func(context.Context, string) (time.Duration, map[string]any, error)

type probeResult struct {
	host    string
	latency time.Duration
	startup map[string]any
	err     error
}

func selectRoute(ctx context.Context, cachedHost string, check probe) (routeResult, error) {
	if cachedHost != "" {
		host, err := normalizeHost(cachedHost)
		if err != nil {
			return routeResult{}, fmt.Errorf("cached JavDB host: %w", err)
		}
		latency, _, err := check(ctx, host)
		if err == nil {
			return routeResult{Host: host, Latency: latency, ReusedCached: true}, nil
		}
		if ctx.Err() != nil {
			return routeResult{}, ctx.Err()
		}
	}

	bootstrapResults := probeAll(ctx, bootstrapHosts, check)
	if ctx.Err() != nil {
		return routeResult{}, ctx.Err()
	}

	var dynamicCandidates []string
	var discoveryErrors []error
	for _, result := range bootstrapResults {
		if result.err != nil {
			discoveryErrors = append(discoveryErrors, fmt.Errorf("%s: %w", result.host, result.err))
			continue
		}
		hosts, err := apiHostsFromStartup(result.startup)
		if err != nil {
			discoveryErrors = append(discoveryErrors, fmt.Errorf("%s: %w", result.host, err))
			continue
		}
		if len(hosts) != 0 && len(dynamicCandidates) == 0 {
			dynamicCandidates = hosts
			break
		}
	}

	known := make(map[string]probeResult, len(bootstrapResults))
	for _, result := range bootstrapResults {
		known[result.host] = result
	}
	var pending []string
	for _, host := range dynamicCandidates {
		if _, ok := known[host]; !ok {
			pending = append(pending, host)
		}
	}
	for _, result := range probeAll(ctx, pending, check) {
		known[result.host] = result
	}
	if ctx.Err() != nil {
		return routeResult{}, ctx.Err()
	}

	candidates := make([]string, 0, len(dynamicCandidates)+len(bootstrapHosts))
	seen := make(map[string]bool, cap(candidates))
	for _, host := range append(dynamicCandidates, bootstrapHosts...) {
		if !seen[host] {
			seen[host] = true
			candidates = append(candidates, host)
		}
	}

	var selected probeResult
	for _, host := range candidates {
		result := known[host]
		if result.err != nil {
			continue
		}
		if selected.host == "" || result.latency < selected.latency {
			selected = result
		}
	}
	if selected.host == "" {
		if len(discoveryErrors) == 0 {
			return routeResult{}, errors.New("no JavDB route responded successfully")
		}
		return routeResult{}, fmt.Errorf("select JavDB route: %w", errors.Join(discoveryErrors...))
	}
	return routeResult{Host: selected.host, Latency: selected.latency}, nil
}

func probeAll(ctx context.Context, hosts []string, check probe) []probeResult {
	results := make(chan probeResult, len(hosts))
	for _, host := range hosts {
		go func() {
			latency, startup, err := check(ctx, host)
			results <- probeResult{host: host, latency: latency, startup: startup, err: err}
		}()
	}

	collected := make([]probeResult, 0, len(hosts))
	for range hosts {
		select {
		case <-ctx.Done():
			return collected
		case result := <-results:
			collected = append(collected, result)
		}
	}
	return collected
}

func apiHostsFromStartup(startup map[string]any) ([]string, error) {
	raw, exists := startup["backup_domains_data"]
	if !exists {
		return nil, nil
	}
	encoded, ok := raw.(string)
	if !ok {
		return nil, errors.New("startup backup_domains_data is not a string")
	}
	payload, err := decryptBackupDomains(encoded)
	if err != nil {
		return nil, err
	}
	rawDomains, exists := payload["apiDomains"]
	if !exists {
		return nil, nil
	}
	domains, ok := rawDomains.([]any)
	if !ok {
		return nil, errors.New("backup domains has no apiDomains")
	}

	hosts := make([]string, 0, len(domains))
	seen := make(map[string]bool, len(domains))
	for _, item := range domains {
		value, ok := item.(string)
		if !ok {
			return nil, errors.New("apiDomains contains a non-string value")
		}
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

func decryptBackupDomains(encoded string) (map[string]any, error) {
	encrypted, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode backup domains: %w", err)
	}
	if len(encrypted) == 0 || len(encrypted)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("decode backup domains: invalid cipher length %d", len(encrypted))
	}

	block, err := aes.NewCipher(backupKey)
	if err != nil {
		return nil, fmt.Errorf("create backup domains cipher: %w", err)
	}
	plain := make([]byte, len(encrypted))
	cipher.NewCBCDecrypter(block, backupIV).CryptBlocks(plain, encrypted)
	plain, err = unpadPKCS7(plain)
	if err != nil {
		return nil, fmt.Errorf("decode backup domains: %w", err)
	}
	if !utf8.Valid(plain) {
		return nil, errors.New("decode backup domains: invalid UTF-8")
	}

	var payload map[string]any
	if err := json.Unmarshal(plain, &payload); err != nil {
		return nil, fmt.Errorf("decode backup domains JSON: %w", err)
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
