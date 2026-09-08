package pan

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestPlayURLDecodesOfficialResponses(t *testing.T) {
	for _, test := range []struct {
		name    string
		hls     bool
		body    string
		wantErr bool
	}{
		{name: "original", body: `{"state":true,"code":0,"data":{"42":{"url":{"url":"https://cdn.example/video?sign=fixture"}}}}`},
		{name: "hls without extension", hls: true, body: `{"state":true,"code":0,"data":{"video_url":[{"url":"http://cdn.example/m3u8/fixture?definition=4","height":1080,"width":1920,"definition":4,"title":1080}]}}`},
		{name: "missing download", body: `{"state":true,"code":0,"data":{}}`, wantErr: true},
		{name: "empty download URL", body: `{"state":true,"code":0,"data":{"42":{"url":{"url":""}}}}`, wantErr: true},
		{name: "transcode pending", hls: true, body: `{"state":true,"code":0,"data":{"video_url":[]}}`, wantErr: true},
		{name: "missing hls URL", hls: true, body: `{"state":true,"code":0,"data":{"video_url":[{"height":1080}]}}`, wantErr: true},
		{name: "wrong wire type", hls: true, body: `{"state":true,"code":0,"data":{"video_url":{"url":"https://cdn.example/video"}}}`, wantErr: true},
		{name: "api rejection", hls: true, body: `{"state":false,"code":500001,"message":"fixture failure"}`, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := New(Options{})
			defer client.Close()
			calls := 0
			client.http.SetTransport(offlineRoundTrip(func(request *http.Request) (*http.Response, error) {
				calls++
				if request.UserAgent() != mediaUserAgent || request.Header.Get("Authorization") != "Bearer fixture-token" {
					t.Error("play URL request must use the media UA and OAuth token")
				}
				if test.hls {
					if request.Method != http.MethodGet || request.URL.Path != "/open/video/play" || request.URL.Query().Get("pick_code") != "fixture-pick" {
						t.Errorf("unexpected HLS request: %s %s", request.Method, request.URL.Path)
					}
				} else {
					if err := request.ParseForm(); err != nil {
						t.Fatal(err)
					}
					if request.Method != http.MethodPost || request.URL.Path != "/open/ufile/downurl" || request.PostForm.Get("pick_code") != "fixture-pick" {
						t.Errorf("unexpected download URL request: %s %s", request.Method, request.URL.Path)
					}
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(test.body)), Request: request}, nil
			}))
			sources, err := client.PlayURL(t.Context(), "fixture-token", "fixture-pick", test.hls)
			if (err != nil) != test.wantErr {
				t.Fatalf("PlayURL error = %v, want error = %t", err, test.wantErr)
			}
			if !test.wantErr && (len(sources) != 1 || sources[0].URL == "" || (test.hls && sources[0].Height != 1080)) {
				t.Fatalf("sources = %#v", sources)
			}
			if calls != 1 {
				t.Fatalf("sent %d API calls for one source request", calls)
			}
		})
	}
}

type observedMediaBody struct {
	io.Reader
	reads  int
	closed bool
}

func (body *observedMediaBody) Read(buffer []byte) (int, error) {
	body.reads++
	return body.Reader.Read(buffer)
}

func (body *observedMediaBody) Close() error {
	body.closed = true
	return nil
}

func TestOpenMediaStreamsRangeAndHeadWithoutCredentials(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			client := New(Options{})
			defer client.Close()
			body := &observedMediaBody{Reader: strings.NewReader("fixture video")}
			client.media.SetTransport(offlineRoundTrip(func(request *http.Request) (*http.Response, error) {
				if request.Method != method || request.UserAgent() != mediaUserAgent || request.Header.Get("Range") != "bytes=2-5" || request.Header.Get("If-Range") != `"fixture-etag"` {
					t.Errorf("method or media headers were lost: %s %#v", request.Method, request.Header)
				}
				if request.Header.Get("Authorization") != "" || request.Header.Get("Cookie") != "" {
					t.Error("browser credentials were forwarded to the CDN")
				}
				return &http.Response{StatusCode: http.StatusPartialContent, Header: http.Header{"Content-Range": {"bytes 2-5/13"}}, Body: body, Request: request}, nil
			}))
			response, err := client.OpenMedia(t.Context(), method, "https://cdn.example/video?sign=fixture", http.Header{
				"Range": {"bytes=2-5"}, "If-Range": {`"fixture-etag"`}, "Authorization": {"Bearer private"}, "Cookie": {"private=value"},
			})
			if err != nil {
				t.Fatal(err)
			}
			if body.reads != 0 || response.StatusCode != http.StatusPartialContent || response.Header.Get("Content-Range") != "bytes 2-5/13" {
				t.Fatal("OpenMedia buffered the response or lost its range status")
			}
			if client.media.GetClient().Timeout != 0 {
				t.Fatal("long video transfers must not inherit the API total timeout")
			}
			response.Body.Close()
			if !body.closed {
				t.Fatal("caller could not close the upstream body")
			}
		})
	}
}

func TestMediaRequestErrorRedactsURLAndPreservesCancellation(t *testing.T) {
	client := New(Options{})
	defer client.Close()
	client.media.SetTransport(offlineRoundTrip(func(*http.Request) (*http.Response, error) {
		return nil, context.Canceled
	}))
	_, err := client.OpenMedia(t.Context(), http.MethodGet, "https://cdn.example/video?secret=fixture", nil)
	if !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "cdn.example") {
		t.Fatalf("unsafe or unrecognizable media error: %v", err)
	}
}
