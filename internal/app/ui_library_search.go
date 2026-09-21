package app

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"time"
)

func (app *UIApp) handleLibrarySearchSuggestions(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeJSON(writer, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if !requireSource(writer, request.Context(), sourceHongguo) {
		return
	}
	query, err := hongguoSearchKeyword(request.URL.Query().Get("q"))
	if err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	items, err := app.downloader.hongguoSearchSuggestions(request.Context(), query)
	if request.Context().Err() != nil {
		return
	}
	if err != nil {
		writeJSON(writer, http.StatusBadGateway, map[string]string{"error": "搜索联想暂不可用，仍可按回车搜索"})
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"query": query, "source": sourceHongguo, "data": items})
}

func (app *UIApp) handleLibrarySearch(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeJSON(writer, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	keyword, err := hongguoSearchKeyword(request.URL.Query().Get("q"))
	if err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if !requireSource(writer, request.Context(), sourceHongguo) {
		return
	}
	if request.URL.Query().Get("stream") == "1" {
		app.handleLibrarySearchStream(writer, request, keyword)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 45*time.Second)
	defer cancel()
	result, err := app.downloader.searchHongguoDramas(ctx, keyword)
	if err != nil {
		if request.Context().Err() != nil {
			return
		}
		fmt.Printf("红果联网搜索失败: %v\n", publicError(err))
		writeJSON(writer, http.StatusBadGateway, map[string]string{"error": "联网搜索暂不可用，请稍后重试；本地筛选仍可使用"})
		return
	}
	if ctx.Err() != nil {
		return
	}
	dramas := app.importLibrarySearchDramas(result.Dramas)
	if len(dramas) > 0 {
		app.persistLibrary()
	}
	app.mu.Lock()
	saved := app.librarySaved && !app.libraryDirty
	app.mu.Unlock()
	writeJSON(writer, http.StatusOK, map[string]any{
		"query": keyword, "source": sourceHongguo, "data": dramas, "total": result.Total,
		"limited": result.Limited, "warning": result.Warning, "saved": saved,
	})
}

func (app *UIApp) importLibrarySearchDramas(items []Drama) []Drama {
	dramas := make([]Drama, 0, len(items))
	for _, drama := range items {
		source, id, valid := splitProviderDramaID(drama.ID)
		if valid && source == sourceHongguo && hongguoNumericID.MatchString(id) && (drama.Source == "" || canonicalProviderSource(drama.Source) == sourceHongguo) {
			dramas = append(dramas, drama)
		}
	}
	if len(dramas) > 0 {
		positions := make(map[string]int, len(dramas))
		for index, drama := range dramas {
			positions[drama.ID] = index
		}
		app.mu.Lock()
		app.normalizeDramaCovers(dramas)
		known := make(map[string]Drama, len(dramas))
		for _, drama := range app.dramas {
			if _, found := positions[drama.ID]; found {
				known[drama.ID] = drama
			}
		}
		changed := app.librarySources[sourceHongguo].Status == ""
		for index, drama := range dramas {
			previous, found := known[drama.ID]
			if found {
				drama = mergeDramaMetadata(drama, previous)
			}
			changed = changed || !found || !reflect.DeepEqual(drama, previous)
			dramas[index] = drama
		}
		if !changed {
			app.mu.Unlock()
			return dramas
		}
		newDramas := app.newSortMetadataDramasLocked(dramas)
		app.dramas = mergeSourceDramas(app.dramas, dramas, nil, sourceHongguo)
		app.enqueueSortMetadataLocked(newDramas, false)
		app.loadedAt = time.Now()
		if app.librarySources == nil {
			app.librarySources = map[string]librarySourceState{}
		}
		state := app.librarySources[sourceHongguo]
		state.Count = 0
		for _, drama := range app.dramas {
			if dramaProvider(drama) == sourceHongguo {
				state.Count++
			}
			if index, found := positions[drama.ID]; found {

				dramas[index] = drama
			}
		}
		state.UpdatedAt = app.loadedAt
		if state.Status == "" {
			state.Status = "ready"
		}
		app.librarySources[sourceHongguo] = state
		app.libraryDirty = true
		app.libraryRevision++
		app.mu.Unlock()
	}
	return dramas
}
