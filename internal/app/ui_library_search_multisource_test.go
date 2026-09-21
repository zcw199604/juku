package app

import (
	"reflect"
	"testing"
)

func TestLibrarySearchImportPreservesOtherSources(t *testing.T) {
	others := []Drama{
		{ID: "huangdou:existing", Source: sourceHuangdou, Title: "黄豆原有剧集"},
		{ID: "huangguoai:existing", Source: sourceHuangguoAI, Title: "黄果原有剧集"},
	}
	app := &UIApp{dramas: append([]Drama{}, others...), metadataClosed: true}
	result := app.importLibrarySearchDramas([]Drama{
		{ID: "hongguo:700001", Source: sourceHongguo, Title: "红果搜索结果"},
		{ID: "huangdou:existing", Source: sourceHuangdou, Title: "不得覆盖黄豆"},
		{ID: "huangguoai:existing", Source: sourceHuangguoAI, Title: "不得覆盖黄果"},
		{ID: "hongguo:700002", Source: sourceHuangdou, Title: "冲突站源"},
	})
	if len(result) != 1 || result[0].ID != "hongguo:700001" || len(app.dramas) != 3 {
		t.Fatalf("search imported unsupported sources or removed existing dramas: %+v", app.dramas)
	}
	for _, expected := range others {
		found := false
		for _, actual := range app.dramas {
			if actual.ID == expected.ID {
				found = reflect.DeepEqual(actual, expected)
			}
		}
		if !found {
			t.Fatalf("search changed another source: %s", expected.ID)
		}
	}
}
