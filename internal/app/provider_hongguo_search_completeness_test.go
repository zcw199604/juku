package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const hongguoSeasonSearchQuery = "无限叠加攻击"
const hongguoSeasonSearchTitle = "普通弓箭手？我能无限叠加攻击力"

var hongguoSeasonSearchIDs = []string{"7648238378667740184", "7660841363813960729", "7668626435891809342", "7676092957392374846", "7680168711512132633"}
var hongguoSeasonSearchCounts = []int{60, 77, 74, 108, 117}

func hongguoSearchCompletenessFixtures() (string, string) {
	var seasons, names, rows []any
	for i, id := range hongguoSeasonSearchIDs {
		title := hongguoSeasonSearchTitle + []string{"", "第二季", "第三季", "第四季", "第五季"}[i]
		video := map[string]any{"series_id": json.Number(id), "series_title": title, "episode_cnt": hongguoSeasonSearchCounts[i], "category_name": "漫剧", "tags": []string{"冒险"}, "hot_score_data": map[string]any{"score": 100 + i}}
		seasons = append(seasons, map[string]any{"video_data": video})
		names = append(names, map[string]any{"name": title, "word_type": "short_play_name", "keyword": id, "video_data": video})
	}

	rows = append(rows, seasons[0], seasons[1], seasons[3])
	for i := 0; i < 7; i++ {
		rows = append(rows, map[string]any{"video_data": map[string]any{"series_id": fmt.Sprintf("70000000000000001%02d", i), "series_title": fmt.Sprintf("相关剧集%d", i), "series_intro": "冒险相关内容", "category_name": "短剧", "hot_score_data": map[string]any{"score": 9000 + i}}})
	}

	for i := 0; i < 15; i++ {
		id := fmt.Sprintf("70000000000000002%02d", i)
		names = append(names, map[string]any{"name": fmt.Sprintf("其他名称%d", i), "word_type": "short_play_name", "video_data": map[string]any{"series_id": json.Number(id), "series_title": fmt.Sprintf("其他名称%d", i)}})
	}
	names = append(names, names[1],
		map[string]any{"name": "无限叠加攻击小说", "word_type": "title", "keyword": "7358413860274965529", "video_data": map[string]any{"series_id": "7358413860274965529"}},
		map[string]any{"name": hongguoSeasonSearchTitle + "第六季", "word_type": "short_play_name", "keyword": "7681547176396213272"},
		map[string]any{"name": "没有剧集元数据", "word_type": "short_play_name", "keyword": "7000000000000000999", "video_data": map[string]any{"episode_cnt": 10}},
	)
	page, _ := json.Marshal(map[string]any{"loaderData": map[string]any{"search_(keyword)/page": map[string]any{"query": hongguoSeasonSearchQuery, "isSuccess": true, "totalCount": 203, "searchList": rows}}})
	suggestions, _ := json.Marshal(map[string]any{"suggest_list": names})
	return "<script>window._ROUTER_DATA=" + string(page) + "</script>", string(suggestions)
}

func TestHongguoSearchCompletesMissingSeasonsBeyondFirstTen(t *testing.T) {
	page, names := hongguoSearchCompletenessFixtures()
	var pageCalls, nameCalls atomic.Int32
	d := rankingTestDownloader(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/search/" + hongguoSeasonSearchQuery:
			pageCalls.Add(1)
			return rankingHTTPResponse(r, 200, page), nil
		case "/incent_resource/suggestion":
			nameCalls.Add(1)
			if q := r.URL.Query(); q.Get("count") != "50" || q.Get("query") != hongguoSeasonSearchQuery || q.Get("app_id") != "8662" {
				t.Error("formal name search inherited the autocomplete limit or wrong keyword", r.URL)
			}
			return rankingHTTPResponse(r, 200, names), nil
		default:
			t.Error("search unexpectedly fetched details or playback", r.URL.Path)
			return rankingHTTPResponse(r, 404, ""), nil
		}
	})
	result, err := d.searchHongguoDramas(context.Background(), hongguoSeasonSearchQuery)
	if err != nil || len(result.Dramas) != 27 || result.Total != 203 || !result.Limited || result.Warning != "" {
		t.Fatalf("wrong search coverage: %d / %d, limited=%v warning=%q err=%v", len(result.Dramas), result.Total, result.Limited, result.Warning, err)
	}
	byID := map[string]Drama{}
	for i, drama := range result.Dramas {
		if _, seen := byID[drama.ID]; seen {
			t.Fatal("duplicate ID", drama.ID)
		}
		byID[drama.ID] = drama
		if i < 5 && !strings.Contains(drama.Title, hongguoSeasonSearchQuery) {
			t.Error("broadly related show ranked before title matches", drama.Title)
		}
	}
	for i, id := range hongguoSeasonSearchIDs {
		if got := fmt.Sprint(byID["hongguo:"+id].EpisodeCount); got != fmt.Sprint(hongguoSeasonSearchCounts[i]) {
			t.Errorf("missing/corrupt season %d: ID %s, episodes %s", i+1, id, got)
		}
	}
	for _, id := range []string{"7358413860274965529", "7681547176396213272", "7000000000000000999"} {
		if _, found := byID["hongguo:"+id]; found {
			t.Error("novel or unverified candidate became a drama", id)
		}
	}

	result.Dramas[0].Title, result.Dramas[0].Tags[0] = "改动", "改动"
	result, err = d.searchHongguoDramas(context.Background(), " "+hongguoSeasonSearchQuery+" ")
	if err != nil || result.Dramas[0].Title == "改动" || result.Dramas[0].Tags[0] == "改动" || pageCalls.Load() != 1 || nameCalls.Load() != 1 {
		t.Fatal("combined search cache was not isolated or reused", err, pageCalls.Load(), nameCalls.Load())
	}
}

func TestHongguoSearchKeepsIdenticalTitlesWithDifferentIDs(t *testing.T) {
	d := rankingTestDownloader(t, func(r *http.Request) (*http.Response, error) {
		if strings.HasPrefix(r.URL.Path, "/search/") {
			return rankingHTTPResponse(r, 200, `<script>window._ROUTER_DATA={"loaderData":{"search_(keyword)/page":{"query":"同名","isSuccess":true,"totalCount":0,"searchList":[]}}}</script>`), nil
		}
		return rankingHTTPResponse(r, 200, `{"suggest_list":[{"name":"同名","word_type":"short_play_name","video_data":{"series_id":7668626435891809342}},{"name":"同名","word_type":"short_play_name","video_data":{"series_id":7680168711512132633}}]}`), nil
	})
	result, err := d.searchHongguoDramas(context.Background(), "同名")
	if err != nil || len(result.Dramas) != 2 || result.Dramas[0].ID == result.Dramas[1].ID {
		t.Fatal("titles must not be used as drama IDs", result, err)
	}
}

func TestHongguoSearchPartialFailureRetainsResultsAndAllowsRetry(t *testing.T) {
	for _, broken := range []string{"page", "names"} {
		t.Run(broken, func(t *testing.T) {
			page, names := hongguoSearchCompletenessFixtures()
			var failed atomic.Bool
			failed.Store(true)
			d := rankingTestDownloader(t, func(r *http.Request) (*http.Response, error) {
				isPage := strings.HasPrefix(r.URL.Path, "/search/")
				if failed.Load() && (isPage == (broken == "page")) {
					return rankingHTTPResponse(r, 200, `{}`), nil
				}
				if isPage {
					return rankingHTTPResponse(r, 200, page), nil
				}
				return rankingHTTPResponse(r, 200, names), nil
			})
			result, err := d.searchHongguoDramas(context.Background(), hongguoSeasonSearchQuery)
			if err != nil || len(result.Dramas) == 0 || result.Warning == "" || !result.Limited || len(d.hongguoClient().searches) != 0 {
				t.Fatal("partial results were lost or cached as a complete success", err)
			}
			failed.Store(false)
			result, err = d.searchHongguoDramas(context.Background(), hongguoSeasonSearchQuery)
			if err != nil || len(result.Dramas) != 27 || result.Warning != "" {
				t.Fatal("retry did not complete the failed search branch", err)
			}
		})
	}
}

func TestHongguoSearchFailuresDoNotBecomeEmptySuccess(t *testing.T) {
	for _, both := range []bool{false, true} {
		d := rankingTestDownloader(t, func(r *http.Request) (*http.Response, error) {
			if !both && r.URL.Path == "/incent_resource/suggestion" {
				return rankingHTTPResponse(r, 200, `{"suggest_list":[]}`), nil
			}
			return rankingHTTPResponse(r, 200, `{}`), nil
		})
		if _, err := d.searchHongguoDramas(context.Background(), "测试"); err == nil || len(d.hongguoClient().searches) > 0 {
			t.Fatal("failed search was reported/cached as a valid empty result")
		}
	}
}

func TestHongguoSearchConcurrentRequestsShareBothBranches(t *testing.T) {
	page, names := hongguoSearchCompletenessFixtures()
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	d := rankingTestDownloader(t, func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		if strings.HasPrefix(r.URL.Path, "/search/") {
			return rankingHTTPResponse(r, 200, page), nil
		}
		return rankingHTTPResponse(r, 200, names), nil
	})
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if result, err := d.searchHongguoDramas(context.Background(), hongguoSeasonSearchQuery); err != nil || len(result.Dramas) != 27 {
				t.Error("concurrent search failed", err)
			}
		}()
	}
	<-started
	close(release)
	group.Wait()
	if calls.Load() != 2 {
		t.Fatal("identical searches did not share both upstream calls", calls.Load())
	}
}

func TestHongguoSearchCanceledOwnerDoesNotStrandAnotherBrowser(t *testing.T) {
	page, names := hongguoSearchCompletenessFixtures()
	type ownerKey struct{}
	var calls atomic.Int32
	started := make(chan struct{})
	var once sync.Once
	d := rankingTestDownloader(t, func(r *http.Request) (*http.Response, error) {
		if r.Context().Value(ownerKey{}) == true {
			once.Do(func() { close(started) })
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		calls.Add(1)
		if strings.HasPrefix(r.URL.Path, "/search/") {
			return rankingHTTPResponse(r, 200, page), nil
		}
		return rankingHTTPResponse(r, 200, names), nil
	})
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), ownerKey{}, true))
	defer cancel()
	owner := make(chan error, 1)
	go func() { _, err := d.searchHongguoDramas(ctx, hongguoSeasonSearchQuery); owner <- err }()
	<-started
	waiter := make(chan error, 1)
	go func() {
		result, err := d.searchHongguoDramas(context.Background(), hongguoSeasonSearchQuery)
		if err == nil && len(result.Dramas) != 27 {
			err = fmt.Errorf("wrong number of results: %d", len(result.Dramas))
		}
		waiter <- err
	}()
	cancel()
	if err := <-owner; !errors.Is(err, context.Canceled) {
		t.Fatal("canceled search did not stop", err)
	}
	if err := <-waiter; err != nil || calls.Load() != 2 {
		t.Fatal("waiting browser inherited cancellation", err, calls.Load())
	}
}

func TestLibrarySearchCompletenessResponseAndPersistence(t *testing.T) {
	page, names := hongguoSearchCompletenessFixtures()
	d := rankingTestDownloader(t, func(r *http.Request) (*http.Response, error) {
		if strings.HasPrefix(r.URL.Path, "/search/") {
			return rankingHTTPResponse(r, 200, page), nil
		}
		return rankingHTTPResponse(r, 200, names), nil
	})
	a := &UIApp{downloader: d, cfg: d.cfg, metadataClosed: true}
	w := httptest.NewRecorder()
	a.handleLibrarySearch(w, httptest.NewRequest(http.MethodGet, "/api/ui/search?q="+url.QueryEscape(hongguoSeasonSearchQuery), nil))
	var response struct {
		Query   string  `json:"query"`
		Data    []Drama `json:"data"`
		Limited bool    `json:"limited"`
		Saved   bool    `json:"saved"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || w.Code != 200 || response.Query != hongguoSeasonSearchQuery || len(response.Data) != 27 || !response.Saved || !response.Limited || len(a.dramas) != 27 {
		t.Fatal("search response/import lost completed results", w.Code, err)
	}
}

func TestHongguoSearchNormalizedTitleRanking(t *testing.T) {
	query := hongguoSearchText("  ＡＢＣ：无限 叠加攻击  ")
	if query != "abc无限叠加攻击" || hongguoTitleSearchRank("abc 无限叠加攻击", query) != 0 || hongguoTitleSearchRank("前缀ABC！无限叠加攻击（第五季）", query) != 2 || hongguoTitleSearchRank("无关剧名", query) != 3 {
		t.Fatal("punctuation, spacing or full-width text changed title relevance", query)
	}
	if hongguoSearchText("？！？") == "" {
		t.Fatal("punctuation-only input must not match every title")
	}
}

func TestLiveHongguoSearchCompletesFiveSeasons(t *testing.T) {
	if os.Getenv("JUKU_LIVE_SEARCH_COMPLETENESS") != "1" {
		t.Skip("set JUKU_LIVE_SEARCH_COMPLETENESS=1 for public text-only search verification")
	}
	cfg := defaultConfig()
	cfg.dataDir, cfg.OutputDir, cfg.Retries = t.TempDir(), t.TempDir(), 1
	d := NewDownloader(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := d.searchHongguoDramas(ctx, hongguoSeasonSearchQuery)
	if err != nil || result.Warning != "" {
		t.Fatalf("live search: %v, warning=%s", err, result.Warning)
	}
	byID := map[string]Drama{}
	for _, drama := range result.Dramas {
		byID[drama.SourceID] = drama
	}
	for i, id := range hongguoSeasonSearchIDs {
		drama, found := byID[id]
		if !found {
			t.Errorf("season %d still missing: %s", i+1, id)
			continue
		}
		t.Logf("season=%d, id=%s, episodes=%v, title=%s", i+1, id, drama.EpisodeCount, drama.Title)
	}
	t.Logf("combined entries=%d, website estimate=%d, limited=%v", len(result.Dramas), result.Total, result.Limited)
}
