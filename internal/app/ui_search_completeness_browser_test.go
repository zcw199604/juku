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

func TestSearchCompletenessBrowserFixture(t *testing.T) {
	root := os.Getenv("JUKU_SEARCH_BROWSER_ROOT")
	if root == "" {
		t.Skip("isolated full search browser fixture is opt-in")
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
	page, names := hongguoSearchCompletenessFixtures()
	seasons := hongguoManySeasonFixtures()
	loader := routerLoaderMap(parseRouterData(page), "search_(keyword)/page")
	for _, row := range anyList(loader["searchList"]) {
		a.dramas = append(a.dramas, hongguoDramaFromAny(row, "短剧"))
	}
	for i := 0; i < 5000; i++ {
		a.dramas = append(a.dramas, Drama{ID: fmt.Sprintf("hongguo:71000000000000%05d", i), Source: sourceHongguo, Title: fmt.Sprintf("缓存剧集 %04d", i), CategoryName: "短剧", Desc: "剧库搜索回归测试"})
	}
	initial := append([]Drama{}, a.dramas...)
	a.downloader.client = &http.Client{Transport: rankingTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "hongguoduanju.com" {
			return nil, fmt.Errorf("unexpected fixture host %s", r.URL.Host)
		}
		query := r.URL.Query().Get("query")
		if r.URL.Path == "/incent_resource/suggestion" {
			if body, found := seasons[query]; found && query != "page" {
				if query != hongguoManySeasonQuery {
					select {
					case <-time.After(300 * time.Millisecond):
					case <-r.Context().Done():
						return nil, r.Context().Err()
					}
				}
				return rankingHTTPResponse(r, 200, body), nil
			}
			if query == "慢搜索" {
				select {
				case <-time.After(time.Second):
				case <-r.Context().Done():
					return nil, r.Context().Err()
				}
			}
			if query == "名称故障" || query == "全部故障" {
				return rankingHTTPResponse(r, 200, `{}`), nil
			}
			if query == hongguoSeasonSearchQuery || query == "综合故障" || strings.Contains(query, "弓箭手") {
				return rankingHTTPResponse(r, 200, names), nil
			}
			return rankingHTTPResponse(r, 200, `{"suggest_list":[]}`), nil
		}
		if strings.HasPrefix(r.URL.Path, "/search/") {
			query = strings.TrimPrefix(r.URL.Path, "/search/")
			if query == hongguoManySeasonQuery {
				return rankingHTTPResponse(r, 200, seasons["page"]), nil
			}
			if query == "综合故障" || query == "全部故障" {
				return rankingHTTPResponse(r, 200, `{}`), nil
			}
			if query == hongguoSeasonSearchQuery {
				return rankingHTTPResponse(r, 200, page), nil
			}
			rows := []any{}
			if query != "无结果" {
				id := "7200000000000000001"
				if query == "慢搜索" {
					id = "7200000000000000002"
				}
				rows = append(rows, map[string]any{"video_data": map[string]any{"series_id": id, "series_title": query, "episode_cnt": 12}})
			}
			body, _ := json.Marshal(map[string]any{"loaderData": map[string]any{"search_(keyword)/page": map[string]any{"query": query, "isSuccess": true, "totalCount": len(rows), "searchList": rows}}})
			return rankingHTTPResponse(r, 200, "<script>window._ROUTER_DATA="+string(body)+"</script>"), nil
		}
		return nil, fmt.Errorf("unexpected fixture path %s", r.URL.Path)
	})}
	if err := a.prepareBrowserViewers(); err != nil {
		t.Fatal(err)
	}
	routes := a.routes()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/__search_reset" && r.Method == http.MethodPost {
			a.mu.Lock()
			a.dramas = append([]Drama{}, initial...)
			a.libraryRevision++
			a.mu.Unlock()
			client := a.downloader.hongguoClient()
			client.mu.Lock()
			client.searches = map[string]hongguoSearchEntry{}
			client.mu.Unlock()
			writeJSON(w, 200, map[string]bool{"ok": true})
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
