package app

import (
	"context"
	"encoding/json"
	"html"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func rankingMergedFixture(board rankingBoard, page int, rows []map[string]any) string {
	key := "rank_" + board.path + "/page"
	base := rankingFixture(board, page, rows)
	base = strings.Split(base, "</script>")[0] + "</script>"
	content, _ := json.Marshal(map[string]any{"isSuccess": true, "rankList": rows, "pagination": map[string]int{"pageNum": page, "totalPages": 2}})
	args, _ := json.Marshal([]any{key, []any{map[string]any{"key": "content", "routerDataFnName": "p", "routerDataFnArgs": []string{string(content)}}}})
	return base + `<script data-script-src="modern-run-window-fn" data-fn-name="mergeLoaderData" data-fn-args="` + html.EscapeString(string(args)) + `"></script>`
}

func TestHongguoRankingMergeLoaderFormatsPreserveMetadata(t *testing.T) {
	for _, board := range rankingBoards {
		if board.Source != sourceHongguo {
			continue
		}
		for page := 1; page <= 2; page++ {
			rows := rankingRows(page)
			rows[0]["title"] = `甲 &amp; <乙> "丙" '丁' / 戊`
			rows[0]["scoreText"] = "评分9.0"
			rows[0]["tags"] = []string{"剧情", "成长"}
			rows[0]["description"] = "文字简介 & 内容"
			legacy, err := parseHongguoRanking(rankingFixture(board, page, rows), board, page)
			if err != nil {
				t.Fatal(err)
			}
			merged, err := parseHongguoRanking(rankingMergedFixture(board, page, rows), board, page)
			if err != nil || !reflect.DeepEqual(merged, legacy) {
				t.Fatal("merged and streamed formats differ", board.ID, page, err)
			}
			if merged.Items[0].Drama.TotalEpisode != 2 || merged.Items[0].Drama.Score != "9.0" || merged.Items[0].Drama.Title != rows[0]["title"] {
				t.Fatal("lost episode, score or escaped text metadata")
			}
		}
	}
}

func TestHongguoRankingMergeLoaderRejectsWrongRouteAndInvalidContent(t *testing.T) {
	board, _ := findRankingBoard("hongguo-hot")
	body := rankingMergedFixture(board, 1, rankingRows(1))
	for _, value := range []string{
		strings.Replace(body, `data-fn-name="mergeLoaderData"`, `data-fn-name="unrelated"`, 1),
		strings.Replace(body, `data-script-src="modern-run-window-fn"`, `data-script-src="unrelated"`, 1),
		strings.ReplaceAll(body, `hot-drama/page`, `hot-ai-drama/page`),
		strings.ReplaceAll(body, `routerDataFnArgs`, `unrelatedArgs`),
		strings.ReplaceAll(body, `isSuccess`, `unknownStatus`),
	} {
		if _, err := parseHongguoRanking(value, board, 1); err == nil {
			t.Fatal("unrelated or invalid merge payload was accepted")
		}
	}
	if _, err := parseHongguoRanking(body, board, 2); err == nil {
		t.Fatal("repeated page was accepted")
	}
	rows := rankingRows(1)
	rows[1]["id"], rows[1]["seriesId"] = rows[0]["id"], rows[0]["seriesId"]
	if _, err := parseHongguoRanking(rankingMergedFixture(board, 1, rows), board, 1); err == nil {
		t.Fatal("duplicate merge rows were accepted")
	}
	deferred := `<script data-script-src="modern-run-window-fn" data-fn-name="mergeLoaderData" data-fn-args="[&quot;rank_hot-drama/page&quot;,[{&quot;key&quot;:&quot;content&quot;,&quot;routerDataFnName&quot;:&quot;s&quot;,&quot;routerDataFnArgs&quot;:[]}]]"></script>`
	if _, err := parseHongguoRanking(deferred+rankingFixture(board, 1, rankingRows(1)), board, 1); err != nil {
		t.Fatal("deferred merge prevented streamed content", err)
	}
}

func TestLiveHongguoRankingFormats(t *testing.T) {
	if os.Getenv("JUKU_LIVE_HONGGUO_RANKING") != "1" {
		t.Skip("metadata-only live ranking check is opt-in")
	}
	cfg := defaultConfig()
	cfg.dataDir, cfg.OutputDir, cfg.Retries = t.TempDir(), t.TempDir(), 1
	downloader := NewDownloader(cfg)
	for _, board := range rankingBoards {
		if board.Source != sourceHongguo {
			continue
		}
		t.Run(board.ID, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
			defer cancel()
			for page := 1; page <= 2; page++ {
				result, err := downloader.loadRankingPage(ctx, board, page, true)
				if err != nil || len(result.Items) == 0 || result.Items[0].Rank != (page-1)*20+1 {
					t.Fatalf("page %d failed: %v", page, err)
				}
				t.Logf("page %d: %d entries, rank %d–%d", page, len(result.Items), result.Items[0].Rank, result.Items[len(result.Items)-1].Rank)
				if !result.HasMore {
					break
				}
			}
		})
	}
}
