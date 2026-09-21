package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type librarySearchStreamFrame struct {
	Query   string  `json:"query"`
	Source  string  `json:"source"`
	Data    []Drama `json:"data"`
	Done    bool    `json:"done"`
	Saved   bool    `json:"saved"`
	Limited bool    `json:"limited"`
	Warning string  `json:"warning"`
}

func TestLibrarySearchStreamSharesAndPublishesBeforeCompletion(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	var calls atomic.Int32
	transport := hongguoManySeasonTransport(t, hongguoManySeasonFixtures(), nil)
	d := rankingTestDownloader(t, func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if strings.HasSuffix(r.URL.Query().Get("query"), "第七季") {
			close(entered)
			select {
			case <-release:
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
		}
		return transport(r)
	})
	a := &UIApp{downloader: d, cfg: d.cfg, metadataClosed: true}
	server := httptest.NewServer(http.HandlerFunc(a.handleLibrarySearch))
	defer func() { unblock(); server.Close() }()
	client := &http.Client{Timeout: 5 * time.Second}
	open := func() (*json.Decoder, func()) {
		t.Helper()
		response, err := client.Get(server.URL + "/api/ui/search?stream=1&q=" + url.QueryEscape(hongguoManySeasonQuery))
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != 200 || !strings.Contains(response.Header.Get("Content-Type"), "application/x-ndjson") || response.Header.Get("X-Accel-Buffering") != "no" {
			t.Fatal("response was buffered or not a stream", response.StatusCode, response.Header)
		}
		return json.NewDecoder(response.Body), func() { response.Body.Close() }
	}
	read := func(decoder *json.Decoder) librarySearchStreamFrame {
		t.Helper()
		var frame librarySearchStreamFrame
		if err := decoder.Decode(&frame); err != nil {
			t.Fatal("stream ended before a result was available", err)
		}
		if frame.Query != hongguoManySeasonQuery || frame.Source != sourceHongguo {
			t.Fatal("wrong query/source in batch", frame.Query, frame.Source)
		}
		return frame
	}
	first, closeFirst := open()
	defer closeFirst()
	frame := read(first)
	if frame.Done || len(frame.Data) != 19 {
		t.Fatal("first name batch waited for completion or was truncated", frame.Done, len(frame.Data))
	}
	<-entered
	frame = read(first)
	if frame.Done || len(frame.Data) != 23 {
		t.Fatal("initial page was not delivered before supplement", frame.Done, len(frame.Data))
	}
	a.mu.Lock()
	registered := len(a.dramas)
	a.mu.Unlock()
	if registered != 23 {
		t.Fatal("visible search results were not registered for details/playback", registered)
	}
	second, closeSecond := open()
	defer closeSecond()
	frame = read(second)
	if frame.Done || len(frame.Data) != 23 || calls.Load() != 3 {
		t.Fatal("another browser missed existing progress or duplicated upstream work", frame.Done, len(frame.Data), calls.Load())
	}
	unblock()
	for _, decoder := range []*json.Decoder{first, second} {
		for {
			frame = read(decoder)
			if frame.Done {
				break
			}
		}
		assertHongguoManySeasons(t, frame.Data)
		if !frame.Saved || !frame.Limited || frame.Warning != "" {
			t.Fatal("wrong final completion/persistence state", frame.Saved, frame.Limited, frame.Warning)
		}
	}
	if calls.Load() != 6 {
		t.Fatal("concurrent streams did not share the six upstream requests", calls.Load())
	}
	cache, err := readLibraryCache(a.cfg.dataDirectory())
	if err != nil {
		t.Fatal("stream results were not persisted", err)
	}
	assertHongguoManySeasons(t, cache.Dramas)
}

func TestHongguoSearchCanceledSupplementStopsAndReleasesPendingQuery(t *testing.T) {
	entered := make(chan struct{})
	transport := hongguoManySeasonTransport(t, hongguoManySeasonFixtures(), nil)
	d := rankingTestDownloader(t, func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Query().Get("query"), "第七季") {
			close(entered)
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		return transport(r)
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := d.searchHongguoDramas(ctx, hongguoManySeasonQuery)
		done <- err
	}()
	<-entered
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal("canceled supplement continued or became a completed result", err)
	}
	client := d.hongguoClient()
	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.searchPending) != 0 || len(client.searches) != 0 {
		t.Fatal("canceled supplement stranded a query or cached incomplete data")
	}
}

func TestLibrarySearchStreamCancellationPersistsDeliveredResults(t *testing.T) {
	entered := make(chan struct{})
	transport := hongguoManySeasonTransport(t, hongguoManySeasonFixtures(), nil)
	d := rankingTestDownloader(t, func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Query().Get("query"), "第七季") {
			close(entered)
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		return transport(r)
	})
	a := &UIApp{downloader: d, cfg: d.cfg, metadataClosed: true}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "/api/ui/search?stream=1&q="+url.QueryEscape(hongguoManySeasonQuery), nil).WithContext(ctx)
	writer, finished := httptest.NewRecorder(), make(chan struct{})
	go func() { a.handleLibrarySearch(writer, request); close(finished) }()
	<-entered
	cancel()
	<-finished
	cache, err := readLibraryCache(a.cfg.dataDirectory())
	if err != nil || len(cache.Dramas) != 23 || !writer.Flushed || strings.Contains(writer.Body.String(), `"done":true`) {
		t.Fatal("canceling lost visible results or falsely reported completion", err, len(cache.Dramas), writer.Flushed)
	}
}

func TestLibrarySearchStreamInputAndFailureStatus(t *testing.T) {
	d := rankingTestDownloader(t, func(r *http.Request) (*http.Response, error) { return rankingHTTPResponse(r, 200, `{}`), nil })
	a := &UIApp{downloader: d, cfg: d.cfg, metadataClosed: true}
	for _, test := range []struct {
		method string
		query  string
		status int
	}{
		{http.MethodPost, "修仙", 405},
		{http.MethodGet, "", 400},
		{http.MethodGet, "修仙", 502},
	} {
		writer := httptest.NewRecorder()
		a.handleLibrarySearch(writer, httptest.NewRequest(test.method, "/api/ui/search?stream=1&q="+url.QueryEscape(test.query), nil))
		if writer.Code != test.status || !strings.Contains(writer.Header().Get("Content-Type"), "application/json") {
			t.Fatal("invalid/failed search started a successful stream", writer.Code, test.status)
		}
	}
}
