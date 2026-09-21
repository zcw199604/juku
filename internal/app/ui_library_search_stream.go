package app

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

func (app *UIApp) handleLibrarySearchStream(writer http.ResponseWriter, request *http.Request, keyword string) {
	ctx, cancel := context.WithTimeout(request.Context(), 45*time.Second)
	defer cancel()
	started, imported := false, false
	defer func() {
		if imported {
			app.persistLibrary()
		}
	}()
	encoder, controller := json.NewEncoder(writer), http.NewResponseController(writer)
	defer controller.SetWriteDeadline(time.Time{})
	write := func(payload map[string]any) {
		if ctx.Err() != nil {
			return
		}
		if !started {
			writer.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
			writer.Header().Set("Cache-Control", "no-store")
			writer.Header().Set("X-Accel-Buffering", "no")
			started = true
		}
		_ = controller.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := encoder.Encode(payload); err != nil {
			cancel()
			return
		}
		if err := controller.Flush(); err != nil {
			cancel()
		}
		_ = controller.SetWriteDeadline(time.Time{})
	}
	result, err := app.downloader.searchHongguoDramasProgress(ctx, keyword, func(entry hongguoSearchEntry) {
		if ctx.Err() != nil {
			return
		}
		dramas := app.importLibrarySearchDramas(entry.Dramas)
		imported = imported || len(dramas) > 0
		write(map[string]any{
			"query": keyword, "source": sourceHongguo, "data": dramas, "total": entry.Total,
			"limited": true, "done": false,
		})
	})
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		payload := map[string]any{"query": keyword, "source": sourceHongguo, "done": true, "error": "后续搜索暂不可用，已保留收到的结果，请重试"}
		if started {
			write(payload)
		} else {
			writeJSON(writer, http.StatusBadGateway, payload)
		}
		return
	}
	dramas := app.importLibrarySearchDramas(result.Dramas)
	if imported || len(dramas) > 0 {
		app.persistLibrary()
		imported = false
	}
	app.mu.Lock()
	saved := app.librarySaved && !app.libraryDirty
	app.mu.Unlock()
	write(map[string]any{
		"query": keyword, "source": sourceHongguo, "data": dramas, "total": result.Total,
		"limited": result.Limited, "warning": result.Warning, "saved": saved, "done": true,
	})
}
