package app

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type hongguoSearchEntry struct {
	Dramas    []Drama
	Total     int
	ExpiresAt time.Time
}

type hongguoSearchCall struct {
	done  chan struct{}
	entry hongguoSearchEntry
	err   error
}

func hongguoSearchKeyword(keyword string) (string, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" || !utf8.ValidString(keyword) || utf8.RuneCountInString(keyword) > 80 || strings.IndexFunc(keyword, unicode.IsControl) >= 0 {
		return "", errors.New("请输入 1 至 80 个字符的搜索词")
	}
	return keyword, nil
}

func (downloader *Downloader) searchHongguoDramas(ctx context.Context, keyword string) (hongguoSearchEntry, error) {
	keyword, err := hongguoSearchKeyword(keyword)
	if err != nil {
		return hongguoSearchEntry{}, err
	}
	client := downloader.hongguoClient()
	client.mu.Lock()
	if cached, found := client.searches[keyword]; found && time.Now().Before(cached.ExpiresAt) {
		client.mu.Unlock()
		return cached, nil
	}
	if pending := client.searchPending[keyword]; pending != nil {
		client.mu.Unlock()
		select {
		case <-ctx.Done():
			return hongguoSearchEntry{}, ctx.Err()
		case <-pending.done:
			if (errors.Is(pending.err, context.Canceled) || errors.Is(pending.err, context.DeadlineExceeded)) && ctx.Err() == nil {
				return downloader.searchHongguoDramas(ctx, keyword)
			}
			return pending.entry, pending.err
		}
	}
	pending := &hongguoSearchCall{done: make(chan struct{})}
	client.searchPending[keyword] = pending
	client.mu.Unlock()
	entry, err := downloader.fetchHongguoSearch(ctx, keyword)
	client.mu.Lock()
	if err == nil {
		if len(client.searches) >= 64 {
			client.searches = map[string]hongguoSearchEntry{}
		}
		entry.ExpiresAt = time.Now().Add(5 * time.Minute)
		client.searches[keyword] = entry
	}
	pending.entry, pending.err = entry, err
	delete(client.searchPending, keyword)
	close(pending.done)
	client.mu.Unlock()
	return entry, err
}

func (downloader *Downloader) fetchHongguoSearch(ctx context.Context, keyword string) (hongguoSearchEntry, error) {
	pageURL := hongguoBaseURL + "/search/" + url.PathEscape(keyword)
	body, err := downloader.fetchProviderText(ctx, pageURL, hongguoBaseURL+"/")
	if err != nil {
		return hongguoSearchEntry{}, err
	}
	page := routerLoaderMap(parseRouterData(body), "search_(keyword)/page", "search_")
	rows, valid := page["searchList"].([]any)
	if page["isSuccess"] != true || !valid || mapString(page, "query") != keyword {
		return hongguoSearchEntry{}, errors.New("红果搜索未返回有效结果")
	}
	entry := hongguoSearchEntry{Dramas: make([]Drama, 0, len(rows))}
	seen := make(map[string]bool, len(rows))
	for _, row := range rows {
		if len(nestedMap(row, "video_data")) == 0 {
			continue
		}
		drama := hongguoDramaFromAny(row, "短剧")
		if drama.ID != "" && !seen[drama.ID] {
			seen[drama.ID] = true
			entry.Dramas = append(entry.Dramas, drama)
		}
	}
	if len(rows) > 0 && len(entry.Dramas) == 0 {
		return hongguoSearchEntry{}, errors.New("红果搜索结果中没有可识别的剧集")
	}
	entry.Total, _ = strconv.Atoi(mapString(page, "totalCount"))
	if entry.Total < len(entry.Dramas) {
		entry.Total = len(entry.Dramas)
	}
	return entry, nil
}
