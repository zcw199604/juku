package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const hongguoManySeasonQuery = "谁说没灵根不能修仙的"
const hongguoManySeasonTitle = hongguoManySeasonQuery + "？之无灵证道"

var hongguoManySeasonSuffixes = []string{
	"", "第二季", "第三季", "第四季", "第五季", "第六季", "第七季", "第八季", "第九季", "第十季",
	"第十一季", "第十二季", "第十三季", "第十四季", "第十五季", "第十六季", "第十七季", "第十八季", "第十九季", "第二十季",
	"第21季", "第22季", "第23季", "第24季", "第25季", "第26季", "第27季", "第28季", "第29季", "第30季",
	"第31季", "第32季", "第33季", "第34季",
}

func hongguoManySeasonVideo(season int) map[string]any {
	return map[string]any{
		"series_id":    json.Number(fmt.Sprintf("730000000000000%04d", season)),
		"series_title": hongguoManySeasonTitle + hongguoManySeasonSuffixes[season-1],
		"episode_cnt":  70 + season, "category_name": "漫剧", "tags": []string{"修仙"},
		"hot_score_data": map[string]any{"score": 100 * season},
	}
}

func hongguoManySeasonFixtures() map[string]string {
	results := map[string]string{}
	nameBatches := map[string][]int{
		hongguoManySeasonQuery:          {1, 2, 34, 33, 5, 22, 3, 21, 11, 18, 4, 13, 6, 26, 14, 17, 28, 19, 9},
		hongguoManySeasonTitle + "第七季":  {7, 8, 10, 12, 27, 24},
		hongguoManySeasonTitle + "第十五季": {15, 16, 23},
		hongguoManySeasonTitle + "第二十季": {20, 29},
		hongguoManySeasonTitle + "第30季": {30},
	}
	for query, seasons := range nameBatches {
		var records []any
		for _, season := range seasons {
			records = append(records, map[string]any{"name": hongguoManySeasonTitle + hongguoManySeasonSuffixes[season-1], "word_type": "short_play_name", "video_data": hongguoManySeasonVideo(season)})
		}
		if query == hongguoManySeasonQuery {
			records = append(records, map[string]any{"name": "谁说没灵根不能修仙第二季", "word_type": "common_query"})
		} else if strings.HasSuffix(query, "第七季") {
			records = append(records,
				map[string]any{"name": hongguoManySeasonTitle + "第35季", "word_type": "short_play_name", "keyword": "7300000000000000035"},
				map[string]any{"name": "修仙小说", "word_type": "title", "video_data": hongguoManySeasonVideo(31)},
				map[string]any{"name": "其他作品第七季", "word_type": "short_play_name", "video_data": map[string]any{"series_id": "7300000000000000900", "series_title": "其他作品第七季"}},
				map[string]any{"name": hongguoManySeasonTitle + "第七部", "word_type": "short_play_name", "video_data": map[string]any{"series_id": "7300000000000000901", "series_title": hongguoManySeasonTitle + "第七部"}},
			)
		}
		body, _ := json.Marshal(map[string]any{"suggest_list": records})
		results[query] = string(body)
		results[strings.ReplaceAll(query, "？", "?")] = string(body)
	}
	var rows []any
	for _, season := range []int{1, 34, 33, 2, 32, 31, 4, 5, 25} {
		rows = append(rows, map[string]any{"video_data": hongguoManySeasonVideo(season)})
	}
	rows = append(rows, map[string]any{"video_data": map[string]any{"series_id": "7300000000000000999", "series_title": "修仙相关作品"}})
	body, _ := json.Marshal(map[string]any{"loaderData": map[string]any{"search_(keyword)/page": map[string]any{"query": hongguoManySeasonQuery, "isSuccess": true, "totalCount": 233, "searchList": rows}}})
	results["page"] = "<script>window._ROUTER_DATA=" + string(body) + "</script>"
	return results
}

func hongguoManySeasonTransport(t *testing.T, fixtures map[string]string, before func(*http.Request)) rankingTransport {
	t.Helper()
	return func(request *http.Request) (*http.Response, error) {
		if before != nil {
			before(request)
		}
		if request.URL.Host != "hongguoduanju.com" {
			t.Error("search fetched an unrelated host", request.URL.Host)
		}
		query := request.URL.Query().Get("query")
		if request.URL.Path == "/search/"+hongguoManySeasonQuery {
			query = "page"
		} else if request.URL.Path != "/incent_resource/suggestion" {
			t.Error("search fetched something other than text search data", request.URL.Path)
		}
		body, ok := fixtures[query]
		if !ok {
			t.Errorf("unexpected supplemental query %q", query)
			return rankingHTTPResponse(request, 200, `{"suggest_list":[]}`), nil
		}
		return rankingHTTPResponse(request, 200, body), nil
	}
}

func assertHongguoManySeasons(t *testing.T, dramas []Drama) {
	t.Helper()
	if len(dramas) != 35 {
		t.Fatalf("want 34 seasons and the original related result, got %d", len(dramas))
	}
	byID := map[string]Drama{}
	for _, drama := range dramas {
		if _, found := byID[drama.ID]; found || drama.Source != sourceHongguo {
			t.Fatal("duplicate or wrong source", drama.ID, drama.Source)
		}
		byID[drama.ID] = drama
	}
	for season, suffix := range hongguoManySeasonSuffixes {
		id := fmt.Sprintf("hongguo:730000000000000%04d", season+1)
		drama := byID[id]
		if drama.Title != hongguoManySeasonTitle+suffix || fmt.Sprint(drama.EpisodeCount) != fmt.Sprint(71+season) || drama.Heat != fmt.Sprint(100*(season+1)) {
			t.Errorf("season %d missing or metadata corrupted: %s / %v / %s", season+1, drama.Title, drama.EpisodeCount, drama.Heat)
		}
		if drama.Views != "" || drama.OnlineDate != "" {
			t.Error("missing sorting metadata was invented", drama.ID)
		}
	}
}

func TestHongguoSearchCompletesThirtyFourSeasonsInAdaptiveBatches(t *testing.T) {
	var calls atomic.Int32
	d := rankingTestDownloader(t, hongguoManySeasonTransport(t, hongguoManySeasonFixtures(), func(r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/incent_resource/suggestion" && r.URL.Query().Get("count") != "50" {
			t.Error("formal searches must not inherit the autocomplete limit")
		}
	}))
	var counts []int
	var first []Drama
	result, err := d.searchHongguoDramasProgress(context.Background(), hongguoManySeasonQuery, func(entry hongguoSearchEntry) {
		counts = append(counts, len(entry.Dramas))
		if first == nil {
			first = entry.Dramas
		}
	})
	if err != nil || result.Warning != "" || result.Total != 233 || !result.Limited {
		t.Fatal("wrong completion state", err, result.Warning, result.Total, result.Limited)
	}
	assertHongguoManySeasons(t, result.Dramas)
	if !reflect.DeepEqual(counts, []int{19, 23, 29, 32, 34, 35}) || calls.Load() != 6 || len(first) != 19 {
		t.Fatal("batches were delayed, mutated, redundantly queried, or stopped below 50", counts, calls.Load(), len(first))
	}
	result.Dramas[0].Title = "changed"
	result, err = d.searchHongguoDramas(context.Background(), hongguoManySeasonQuery)
	if err != nil || calls.Load() != 6 {
		t.Fatal("completed query was not cached", err, calls.Load())
	}
	assertHongguoManySeasons(t, result.Dramas)
}

func TestHongguoSearchSeasonSuffixes(t *testing.T) {
	for _, test := range []struct {
		title string
		base  string
		unit  string
		count int
	}{
		{"无灵证道第34季", "无灵证道", "季", 34},
		{"无灵证道（第二十季）", "无灵证道", "季", 20},
		{"无灵证道 第 ３４ 季！", "无灵证道", "季", 34},
		{"无灵证道第两季", "无灵证道", "季", 2},
		{"无灵证道第一百零一部", "无灵证道", "部", 101},
		{"无灵证道第二〇季", "无灵证道", "季", 20},
		{"无灵证道第0季", "", "", 0},
		{"无灵证道第1.5季", "", "", 0},
		{"无灵证道第34季后传", "", "", 0},
		{"无灵证道第9999999999999999999999季", "", "", 0},
		{"无灵证道第十百季", "", "", 0},
		{"第三十四季", "", "", 0},
	} {
		base, count, unit, _ := hongguoSearchSeason(test.title)
		if base != test.base || count != test.count || unit != test.unit {
			t.Errorf("%q parsed as %q / %d / %q", test.title, base, count, unit)
		}
	}
}

func TestHongguoSearchSeasonScope(t *testing.T) {
	d := rankingTestDownloader(t, func(r *http.Request) (*http.Response, error) {
		t.Fatal("unrelated, complete, or explicitly selected series was expanded", r.URL)
		return nil, errors.New("unexpected request")
	})
	for _, test := range []struct {
		query  string
		titles []string
	}{
		{"其他剧", []string{"修仙", "修仙第三季"}},
		{"修仙第三季", []string{"修仙", "修仙第三季"}},
		{"修仙", []string{"修仙第34季"}},
		{"修仙", []string{"修仙", "修仙第二季", "修仙第三季"}},
	} {
		entry := hongguoSearchEntry{}
		for index, title := range test.titles {
			entry.Dramas = append(entry.Dramas, Drama{ID: fmt.Sprint(index), Title: title})
		}
		if d.completeHongguoSearchSeasons(context.Background(), test.query, &entry) {
			t.Error("query incorrectly reported missing seasons", test.query)
		}
	}
}

func TestHongguoSearchSeasonFallbackRequiresVerifiedMetadata(t *testing.T) {
	var queries []string
	d := rankingTestDownloader(t, func(r *http.Request) (*http.Response, error) {
		query := r.URL.Query().Get("query")
		queries = append(queries, query)
		if query == "修仙第二季" {
			return rankingHTTPResponse(r, 200, `{"suggest_list":[{"name":"修仙第二季","word_type":"short_play_name","keyword":"7300000000000000002"}]}`), nil
		}
		if query == "修仙第2季" {
			return rankingHTTPResponse(r, 200, `{"suggest_list":[{"name":"修仙第二季","word_type":"short_play_name","video_data":{"series_id":7300000000000000002,"series_title":"修仙第二季","episode_cnt":42}}]}`), nil
		}
		t.Fatal("unexpected query", query)
		return nil, nil
	})
	entry := hongguoSearchEntry{Dramas: []Drama{{ID: "one", Title: "修仙"}, {ID: "three", Title: "修仙第三季"}}}
	if d.completeHongguoSearchSeasons(context.Background(), "修仙", &entry) || len(entry.Dramas) != 3 || !reflect.DeepEqual(queries, []string{"修仙第二季", "修仙第2季"}) {
		t.Fatal("number style fallback failed or an unverified name became a result", queries, len(entry.Dramas))
	}
	if entry.Dramas[2].ID != "hongguo:7300000000000000002" || fmt.Sprint(entry.Dramas[2].EpisodeCount) != "42" {
		t.Fatal("verified metadata lost precision", entry.Dramas[2].ID, entry.Dramas[2].EpisodeCount)
	}
}

func TestHongguoSearchSeasonFailurePreservesBatchesAndAllowsRetry(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	transport := hongguoManySeasonTransport(t, hongguoManySeasonFixtures(), nil)
	d := rankingTestDownloader(t, func(r *http.Request) (*http.Response, error) {
		if fail.Load() && strings.HasSuffix(r.URL.Query().Get("query"), "第十五季") {
			return rankingHTTPResponse(r, 200, `{}`), nil
		}
		return transport(r)
	})
	result, err := d.searchHongguoDramas(context.Background(), hongguoManySeasonQuery)
	if err != nil || len(result.Dramas) != 29 || !result.Limited || !strings.Contains(result.Warning, "季数") || len(d.hongguoClient().searches) != 0 {
		t.Fatal("supplement failure discarded data or was cached as complete", err, len(result.Dramas), result.Warning)
	}
	fail.Store(false)
	result, err = d.searchHongguoDramas(context.Background(), hongguoManySeasonQuery)
	if err != nil || result.Warning != "" {
		t.Fatal("supplement could not be retried", err, result.Warning)
	}
	assertHongguoManySeasons(t, result.Dramas)
}

func TestHongguoSearchSeasonBudgetAndDeadlineKeepPartialResults(t *testing.T) {
	t.Run("requests", func(t *testing.T) {
		var calls int
		d := rankingTestDownloader(t, func(r *http.Request) (*http.Response, error) {
			calls++
			return rankingHTTPResponse(r, 200, `{"suggest_list":[]}`), nil
		})
		entry := hongguoSearchEntry{Dramas: []Drama{{ID: "one", Title: "修仙"}, {ID: "last", Title: "修仙第200季"}}}
		if !d.completeHongguoSearchSeasons(context.Background(), "修仙", &entry) || len(entry.Dramas) != 2 || calls != hongguoSearchSeasonQueries {
			t.Fatal("unresolved gaps were invented, unbounded, or reported complete", calls, len(entry.Dramas))
		}
	})
	t.Run("deadline", func(t *testing.T) {
		d := rankingTestDownloader(t, func(r *http.Request) (*http.Response, error) {
			<-r.Context().Done()
			return nil, r.Context().Err()
		})
		ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
		defer cancel()
		entry := hongguoSearchEntry{Dramas: []Drama{{ID: "one", Title: "修仙"}, {ID: "three", Title: "修仙第三季"}}}
		if !d.completeHongguoSearchSeasons(ctx, "修仙", &entry) || len(entry.Dramas) != 2 || ctx.Err() != nil {
			t.Fatal("supplement exhausted the response deadline or discarded primary results", ctx.Err())
		}
	})
}

func TestLiveHongguoSearchCompletesThirtyFourSeasons(t *testing.T) {
	if os.Getenv("JUKU_LIVE_SEARCH_SEASONS") != "1" {
		t.Skip("set JUKU_LIVE_SEARCH_SEASONS=1 for text-only seasonal search verification")
	}
	cfg := defaultConfig()
	cfg.dataDir, cfg.OutputDir, cfg.Retries = t.TempDir(), t.TempDir(), 1
	d := NewDownloader(cfg)
	started := time.Now()
	result, err := d.searchHongguoDramasProgress(context.Background(), hongguoManySeasonQuery, func(entry hongguoSearchEntry) {
		t.Logf("batch entries=%d after=%s", len(entry.Dramas), time.Since(started).Round(time.Millisecond))
	})
	if err != nil || result.Warning != "" {
		t.Fatal("live seasonal search incomplete", err, result.Warning)
	}
	byTitle := map[string]Drama{}
	for _, drama := range result.Dramas {
		byTitle[drama.Title] = drama
	}
	for index, suffix := range hongguoManySeasonSuffixes {
		drama, found := byTitle[hongguoManySeasonTitle+suffix]
		if !found || !hongguoNumericID.MatchString(drama.SourceID) {
			t.Errorf("season %d has no verified drama metadata", index+1)
		} else {
			t.Logf("season=%d id=%s episodes=%v", index+1, drama.SourceID, drama.EpisodeCount)
		}
	}
	t.Logf("final entries=%d elapsed=%s", len(result.Dramas), time.Since(started).Round(time.Millisecond))
}
