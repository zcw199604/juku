package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSearchSuggestionsBrowserFixture(t *testing.T) {
	root := os.Getenv("JUKU_SUGGESTIONS_BROWSER_ROOT")
	if root == "" {
		t.Skip("isolated search browser fixture is opt-in")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	a := viewerTestApp(t)
	a.loadedAt, a.libraryAttempted, a.metadataClosed = time.Now(), true, true
	a.tasks = map[string]*UITask{}
	a.cond = sync.NewCond(&a.mu)
	a.statePath = filepath.Join(a.cfg.dataDirectory(), "ui-state.json")
	a.cfg.adminUsername, a.cfg.adminPassword, a.cfg.adminPasswordExplicit = "admin", accountFixturePassword, true
	var statsMu sync.Mutex
	suggestionCalls, searchCalls := map[string]int{}, map[string]int{}
	a.downloader.client = &http.Client{Transport: rankingTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "hongguoduanju.com" {
			return nil, fmt.Errorf("unexpected fixture request to %s", r.URL.Host)
		}
		if r.URL.Path == "/incent_resource/suggestion" {
			query := r.URL.Query().Get("query")
			statsMu.Lock()
			suggestionCalls[query]++
			statsMu.Unlock()
			body := hongguoSuggestionFixture
			switch query {
			case "重生":
				body = `{"suggest_list":[{"name":"重生归来"},{"name":"重生之后"}]}`
			case "失败":
				body = `{"upstream":"unavailable"}`
			case "空白":
				body = `{"suggest_list":[]}`
			case "字面":
				body = `{"suggest_list":[{"name":"<img src=x onerror=alert(1)>字面"}]}`
			case "慢":
				select {
				case <-time.After(time.Second):
				case <-r.Context().Done():
					return nil, r.Context().Err()
				}
			}
			return rankingHTTPResponse(r, 200, body), nil
		}
		if strings.HasPrefix(r.URL.Path, "/search/") {
			query := strings.TrimPrefix(r.URL.Path, "/search/")
			statsMu.Lock()
			searchCalls[query]++
			statsMu.Unlock()
			page, _ := json.Marshal(map[string]any{"loaderData": map[string]any{"search_(keyword)/page": map[string]any{
				"query": query, "isSuccess": true, "totalCount": 1,
				"searchList": []any{map[string]any{"video_data": map[string]any{"series_id": "7000000000000000009", "series_title": query}}},
			}}})
			return rankingHTTPResponse(r, 200, "<script>window._ROUTER_DATA="+string(page)+"</script>"), nil
		}
		return nil, fmt.Errorf("unexpected fixture path %s", r.URL.Path)
	})}
	if err := a.prepareBrowserViewers(); err != nil {
		t.Fatal(err)
	}
	routes := a.routes()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/__suggestions_stats" {
			statsMu.Lock()
			defer statsMu.Unlock()
			writeJSON(w, 200, map[string]any{"suggestions": suggestionCalls, "searches": searchCalls})
			return
		}
		routes.ServeHTTP(w, r)
	}))
	defer server.Close()
	body, _ := json.Marshal(map[string]string{"url": server.URL})
	if err := os.WriteFile(filepath.Join(root, "ready.json"), body, 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(10 * time.Minute)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if _, err := os.Stat(filepath.Join(root, "stop")); err == nil {
				return
			}
		case <-deadline.C:
			t.Fatal("search browser fixture timed out")
		}
	}
}
