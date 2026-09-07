package javdb

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Fixed 2x3 synthetic PNG and its CDN envelope, with no online artwork.
const plainPNG = "iVBORw0KGgoAAAANSUhEUgAAAAIAAAADCAYAAAC56t6BAAAAAXNSR0IArs4c6QAAAARnQU1BAACxjwv8YQUAAAAJcEhZcwAADsMAAA7DAcdvqGQAAAAQSURBVBhXY2BgaPjPgBUAACMEAYD1gidoAAAAAElFTkSuQmCC"
const wrappedPNG = "ePEoNj91cmJyeHh4dTEwPCp4eHh6eHh4e3B+eHh4wZKm+Xh4eHkLKj86eNa2ZJF4eHh8Hzk1OXh4yfdzhBl9eHh4cQgwIQt4eHa7eHh2u3m/F9AceHh4aDE8OSxgLxsYGBCAt/hteHhbfHn4jfpfEHh4eHgxPTY81joY+g=="

func mediaFixture(t *testing.T, encoded string) []byte {
	t.Helper()
	body, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestDecodeImagePayload(t *testing.T) {
	for _, test := range []struct {
		name    string
		encoded string
	}{
		{name: "plain PNG", encoded: plainPNG},
		{name: "XOR-wrapped PNG", encoded: wrappedPNG},
	} {
		t.Run(test.name, func(t *testing.T) {
			media, err := decodeImagePayload(mediaFixture(t, test.encoded))
			if err != nil {
				t.Fatal(err)
			}
			if media.ContentType != "image/png" || !bytes.Equal(media.Body, mediaFixture(t, plainPNG)) {
				t.Fatalf("decoded media = %s, %x", media.ContentType, media.Body)
			}
			decoded, err := png.Decode(bytes.NewReader(media.Body))
			if err != nil {
				t.Fatal(err)
			}
			if bounds := decoded.Bounds(); bounds.Dx() != 2 || bounds.Dy() != 3 {
				t.Fatalf("image bounds = %v, want 2x3", bounds)
			}
		})
	}
}

func TestDecodeImagePayloadRejectsNonImages(t *testing.T) {
	for _, raw := range [][]byte{nil, {0x78}, []byte("<html>CDN error</html>"), {0x78, 0x00, 0x01}} {
		if _, err := decodeImagePayload(raw); err == nil {
			t.Errorf("accepted non-image payload %x", raw)
		}
	}
}

func TestFetchMediaUsesImageTransportWithoutAPIRoute(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusNotFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/cover.jpg" || r.URL.RawQuery != "" || r.Header.Get("jdsignature") != "" {
					t.Errorf("unexpected image request: %s, headers %v", r.URL, r.Header)
				}
				w.Header().Set("Content-Type", "binary/octet-stream")
				w.WriteHeader(status)
				_, _ = w.Write(mediaFixture(t, wrappedPNG))
			}))
			t.Cleanup(server.Close)
			client, err := New(Options{DeviceUUID: "00000000-0000-4000-8000-000000000001"})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(client.Close)
			client.media.SetTransport(server.Client().Transport)
			client.selectRoute = func(context.Context, routeSelection) (*routeState, error) {
				t.Fatal("image download must not select an API route")
				return nil, nil
			}

			media, err := client.FetchMedia(t.Context(), server.URL+"/cover.jpg")
			if status != http.StatusOK {
				var responseError *HTTPError
				if !errors.As(err, &responseError) || responseError.StatusCode != status {
					t.Fatalf("error = %v, want HTTP %d", err, status)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if media.ContentType != "image/png" || !bytes.Equal(media.Body, mediaFixture(t, plainPNG)) {
				t.Fatalf("downloaded media = %s, %x", media.ContentType, media.Body)
			}
		})
	}
}
