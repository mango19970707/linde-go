package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetAreaListPairTool(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/getAreaListPair" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("siteid"); got != "28" {
			t.Fatalf("unexpected siteid: %s", got)
		}

		writeJSON(t, w, map[string]any{
			"success": true,
			"data": []map[string]any{
				{"id": 101, "name": "全厂"},
			},
		})
	})
	defer server.Close()

	tool, err := NewGetAreaListPairTool(ToolConfig{
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewGetAreaListPairTool() error = %v", err)
	}

	got, err := tool.InvokableRun(context.Background(), `{"siteid":28}`)
	if err != nil {
		t.Fatalf("InvokableRun() error = %v", err)
	}

	assertJSONEqual(t, `{"success":true,"data":[{"id":101,"name":"全厂"}]}`, got)
}

func TestGetEquipmentListPairTool(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/getEquipmentListPair" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("siteid"); got != "28" {
			t.Fatalf("unexpected siteid: %s", got)
		}
		if got := r.URL.Query().Get("areaid"); got != "101" {
			t.Fatalf("unexpected areaid: %s", got)
		}

		writeJSON(t, w, map[string]any{
			"success": true,
			"data": []map[string]any{
				{"id": 2, "name": "主空压机厂房"},
			},
		})
	})
	defer server.Close()

	tool, err := NewGetEquipmentListPairTool(ToolConfig{
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewGetEquipmentListPairTool() error = %v", err)
	}

	got, err := tool.InvokableRun(context.Background(), `{"siteid":28,"areaid":101}`)
	if err != nil {
		t.Fatalf("InvokableRun() error = %v", err)
	}

	assertJSONEqual(t, `{"success":true,"data":[{"id":2,"name":"主空压机厂房"}]}`, got)
}

func TestQueryWpcompanyPairTool(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/wp/queryWpcompanyPair" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("siteid"); got != "28" {
			t.Fatalf("unexpected siteid: %s", got)
		}

		writeJSON(t, w, map[string]any{
			"success": true,
			"data": []map[string]any{
				{"id": 8, "name": "外协单位"},
			},
		})
	})
	defer server.Close()

	tool, err := NewQueryWpcompanyPairTool(ToolConfig{
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewQueryWpcompanyPairTool() error = %v", err)
	}

	got, err := tool.InvokableRun(context.Background(), `{"siteid":28}`)
	if err != nil {
		t.Fatalf("InvokableRun() error = %v", err)
	}

	assertJSONEqual(t, `{"success":true,"data":[{"id":8,"name":"外协单位"}]}`, got)
}

func TestQueryWptempletsNoContentTool(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/wp/queryWptempletsNoContent" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("siteid"); got != "28" {
			t.Fatalf("unexpected siteid: %s", got)
		}

		writeJSON(t, w, map[string]any{
			"success": true,
			"data": []map[string]any{
				{"id": 5, "wptypeid": 23, "templetname": "标准作业许可证模板"},
			},
		})
	})
	defer server.Close()

	tool, err := NewQueryWptempletsNoContentTool(ToolConfig{
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewQueryWptempletsNoContentTool() error = %v", err)
	}

	got, err := tool.InvokableRun(context.Background(), `{"siteid":28}`)
	if err != nil {
		t.Fatalf("InvokableRun() error = %v", err)
	}

	assertJSONEqual(t, `{"success":true,"data":[{"id":5,"wptypeid":23,"templetname":"标准作业许可证模板"}]}`, got)
}

func TestQueryWptempletToolParsesApplyContent(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/wp/queryWptemplet" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("id"); got != "256" {
			t.Fatalf("unexpected id: %s", got)
		}

		writeJSON(t, w, map[string]any{
			"success": true,
			"data": map[string]any{
				"id":           256,
				"wptypeid":     3,
				"processtype":  1,
				"applycontent": `{"id":0,"sqxx":[{"name":"wpname","value":"测试"}]}`,
			},
		})
	})
	defer server.Close()

	tool, err := NewQueryWptempletTool(ToolConfig{
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewQueryWptempletTool() error = %v", err)
	}

	got, err := tool.InvokableRun(context.Background(), `{"id":256}`)
	if err != nil {
		t.Fatalf("InvokableRun() error = %v", err)
	}

	assertJSONEqual(t, `{
		"success": true,
		"data": {
			"id": 256,
			"wptypeid": 3,
			"processtype": 1,
			"applycontent": "{\"id\":0,\"sqxx\":[{\"name\":\"wpname\",\"value\":\"测试\"}]}",
			"applycontent_object": {
				"id": 0,
				"sqxx": [
					{"name":"wpname","value":"测试"}
				]
			}
		}
	}`, got)
}

func TestUpdateWpInfoToolPostsJSON(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/wp/updateWpInfo" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("unexpected content type: %s", got)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if got := asInt(t, body["id"]); got != 0 {
			t.Fatalf("unexpected id: %d", got)
		}
		if got := asInt(t, body["wptempletid"]); got != 256 {
			t.Fatalf("unexpected wptempletid: %d", got)
		}

		writeJSON(t, w, map[string]any{
			"success": true,
			"data": map[string]any{
				"id": 90001,
			},
		})
	})
	defer server.Close()

	tool, err := NewUpdateWpInfoTool(ToolConfig{
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewUpdateWpInfoTool() error = %v", err)
	}

	got, err := tool.InvokableRun(context.Background(), `{"payload":{"id":0,"wptempletid":256,"wptypeid":3}}`)
	if err != nil {
		t.Fatalf("InvokableRun() error = %v", err)
	}

	assertJSONEqual(t, `{"success":true,"data":{"id":90001}}`, got)
}

func TestLinkexistWpinfoToolPostsJSON(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/wp/linkexistWpinfo" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("unexpected content type: %s", got)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if got := asInt(t, body["id"]); got != 90002 {
			t.Fatalf("unexpected id: %d", got)
		}
		if got := asInt(t, body["fid"]); got != 90001 {
			t.Fatalf("unexpected fid: %d", got)
		}

		writeJSON(t, w, map[string]any{
			"success": true,
			"data":    true,
		})
	})
	defer server.Close()

	tool, err := NewLinkexistWpinfoTool(ToolConfig{
		BaseURL: server.URL,
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewLinkexistWpinfoTool() error = %v", err)
	}

	got, err := tool.InvokableRun(context.Background(), `{"id":90002,"fid":90001}`)
	if err != nil {
		t.Fatalf("InvokableRun() error = %v", err)
	}

	assertJSONEqual(t, `{"success":true,"data":true}`, got)
}

func TestToolUsesDefaultSiteIDWhenOmitted(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("siteid"); got != "28" {
			t.Fatalf("unexpected siteid: %s", got)
		}

		writeJSON(t, w, map[string]any{
			"success": true,
			"data":    []map[string]any{},
		})
	})
	defer server.Close()

	tool, err := NewGetAreaListPairTool(ToolConfig{
		BaseURL: server.URL,
		SiteID:  28,
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewGetAreaListPairTool() error = %v", err)
	}

	if _, err := tool.InvokableRun(context.Background(), `{}`); err != nil {
		t.Fatalf("InvokableRun() error = %v", err)
	}
}

func TestUpdateWpInfoToolRejectsNonObjectPayload(t *testing.T) {
	tool, err := NewUpdateWpInfoTool(ToolConfig{
		BaseURL: "http://example.com",
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewUpdateWpInfoTool() error = %v", err)
	}

	if _, err := tool.InvokableRun(context.Background(), `{"payload":["bad"]}`); err == nil {
		t.Fatal("expected error for non-object payload")
	}
}

func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func assertJSONEqual(t *testing.T, want, got string) {
	t.Helper()

	var wantValue any
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("unmarshal want: %v", err)
	}

	var gotValue any
	if err := json.Unmarshal([]byte(got), &gotValue); err != nil {
		t.Fatalf("unmarshal got: %v, raw=%s", err, got)
	}

	wantBytes, err := json.Marshal(wantValue)
	if err != nil {
		t.Fatalf("marshal want: %v", err)
	}

	gotBytes, err := json.Marshal(gotValue)
	if err != nil {
		t.Fatalf("marshal got: %v", err)
	}

	if string(wantBytes) != string(gotBytes) {
		t.Fatalf("json mismatch\nwant: %s\ngot:  %s", wantBytes, gotBytes)
	}
}

func asInt(t *testing.T, value any) int {
	t.Helper()

	number, ok := value.(float64)
	if !ok {
		t.Fatalf("value %v is not a number", value)
	}

	return int(number)
}
