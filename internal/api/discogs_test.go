package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

// seqTransport serves scripted responses: each RoundTrip consumes the next
// entry; a nil response entry simulates a transport-level failure such as a
// timeout or connection reset.
type seqTransport struct {
	steps []func(req *http.Request) (*http.Response, error)
	calls int
}

func (t *seqTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.calls >= len(t.steps) {
		return nil, errors.New("no more scripted responses")
	}
	step := t.steps[t.calls]
	t.calls++
	return step(req)
}

func jsonResponse(status int, body string, closed *atomic.Int32) *http.Response {
	rc := io.NopCloser(strings.NewReader(body))
	if closed != nil {
		rc = closeCountingBody{ReadCloser: rc, count: closed}
	}
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Body:       rc,
		Header:     make(http.Header),
	}
}

type closeCountingBody struct {
	io.ReadCloser
	count *atomic.Int32
}

func (c closeCountingBody) Close() error {
	c.count.Add(1)
	return c.ReadCloser.Close()
}

const searchResultsJSON = `{"results":[{"id":7717,"title":"Enigma","type":"artist"}]}`

// TestSearchArtistNoPanicWhenDetailRequestFails reproduces the production
// crash: Step 1 (search) succeeds and registers a deferred body closer over
// `resp`; Step 2's request then fails at the transport level, which used to
// reassign `resp` to nil before returning, panicking the deferred closer.
// SearchArtist must return an error instead of panicking.
func TestSearchArtistNoPanicWhenDetailRequestFails(t *testing.T) {
	d := NewDiscogsClient("test-token", "", "")
	d.httpClient = &http.Client{Transport: &seqTransport{
		steps: []func(req *http.Request) (*http.Response, error){
			func(req *http.Request) (*http.Response, error) {
				return jsonResponse(200, searchResultsJSON, nil), nil
			},
			func(req *http.Request) (*http.Response, error) {
				return nil, errors.New("connection reset by peer")
			},
		},
	}}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SearchArtist panicked when detail request failed: %v", r)
		}
	}()

	artist, err := d.SearchArtist(context.Background(), "Enigma", "")
	if err == nil {
		t.Fatal("expected error when detail request fails")
	}
	if artist != nil {
		t.Fatal("expected nil artist on detail failure")
	}
}

// TestSearchArtistClosesEachBodyOnce verifies both responses' bodies are
// closed exactly once on the success path (no leak, no double-close).
func TestSearchArtistClosesEachBodyOnce(t *testing.T) {
	d := NewDiscogsClient("test-token", "", "")
	searchClosed := &atomic.Int32{}
	detailClosed := &atomic.Int32{}
	d.httpClient = &http.Client{Transport: &seqTransport{
		steps: []func(req *http.Request) (*http.Response, error){
			func(req *http.Request) (*http.Response, error) {
				return jsonResponse(200, searchResultsJSON, searchClosed), nil
			},
			func(req *http.Request) (*http.Response, error) {
				return jsonResponse(200, `{"name":"Enigma","profile":"plain bio","images":[]}`, detailClosed), nil
			},
		},
	}}

	artist, err := d.SearchArtist(context.Background(), "Enigma", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if artist == nil || artist.Name != "Enigma" {
		t.Fatalf("unexpected artist: %+v", artist)
	}
	if got := searchClosed.Load(); got != 1 {
		t.Errorf("search body closed %d times, want 1", got)
	}
	if got := detailClosed.Load(); got != 1 {
		t.Errorf("detail body closed %d times, want 1", got)
	}
}
