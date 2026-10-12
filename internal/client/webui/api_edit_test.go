// internal/client/webui/api_edit_test.go
package webui

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// doJSON sends a request with an optional JSON body and returns status plus raw body.
func doJSON(t *testing.T, method, url, body string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

func TestAPI_UpdateTag(t *testing.T) {
	db := setupAPIDB(t)
	srv := startTestServer(t, db)

	code, raw := doJSON(t, http.MethodPut, srv.URL()+"/api/tags/1", `{"name":"Deep Work","color":"#123456"}`)
	if code != 200 {
		t.Fatalf("want 200, got %d: %s", code, raw)
	}
	var name, color string
	if err := db.QueryRow(`SELECT name, color FROM tags WHERE id = 1`).Scan(&name, &color); err != nil {
		t.Fatal(err)
	}
	if name != "Deep Work" || color != "#123456" {
		t.Fatalf("tag not updated: %q %q", name, color)
	}
}

func TestAPI_UpdateTag_Validation(t *testing.T) {
	db := setupAPIDB(t)
	srv := startTestServer(t, db)

	cases := []struct {
		name string
		url  string
		body string
		want int
	}{
		{"blank name", "/api/tags/1", `{"name":"  ","color":"#fff"}`, 400},
		{"missing color", "/api/tags/1", `{"name":"x"}`, 400},
		{"bad id", "/api/tags/abc", `{"name":"x","color":"#fff"}`, 400},
		{"missing tag", "/api/tags/999", `{"name":"x","color":"#fff"}`, 404},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if code, raw := doJSON(t, http.MethodPut, srv.URL()+c.url, c.body); code != c.want {
				t.Fatalf("want %d, got %d: %s", c.want, code, raw)
			}
		})
	}
}

func TestAPI_UpdateTag_DuplicateName(t *testing.T) {
	db := setupAPIDB(t)
	srv := startTestServer(t, db)
	if _, err := db.Exec(`INSERT INTO tags (id, name, color, created_at) VALUES (2, 'Other', '#000', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}

	code, _ := doJSON(t, http.MethodPut, srv.URL()+"/api/tags/2", `{"name":"Dev","color":"#000"}`)
	if code != http.StatusConflict {
		t.Fatalf("want 409, got %d", code)
	}
}

func TestAPI_UpdateTagRule(t *testing.T) {
	db := setupAPIDB(t)
	srv := startTestServer(t, db)
	if _, err := db.Exec(`INSERT INTO tag_rules (id, tag_id, app_pattern, title_pattern, created_at)
		VALUES (5, 1, 'firefox', 'github', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}

	code, raw := doJSON(t, http.MethodPut, srv.URL()+"/api/tag-rules/5",
		`{"tag_id":1,"app_pattern":" code ","title_pattern":""}`)
	if code != 200 {
		t.Fatalf("want 200, got %d: %s", code, raw)
	}
	var app string
	var title any
	if err := db.QueryRow(`SELECT app_pattern, title_pattern FROM tag_rules WHERE id = 5`).Scan(&app, &title); err != nil {
		t.Fatal(err)
	}
	if app != "code" || title != nil {
		t.Fatalf("rule not updated: app=%q title=%v", app, title)
	}
}

func TestAPI_UpdateTagRule_Validation(t *testing.T) {
	db := setupAPIDB(t)
	srv := startTestServer(t, db)
	if _, err := db.Exec(`INSERT INTO tag_rules (id, tag_id, app_pattern, created_at)
		VALUES (5, 1, 'firefox', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}

	if code, _ := doJSON(t, http.MethodPut, srv.URL()+"/api/tag-rules/5", `{"tag_id":1,"app_pattern":""}`); code != 400 {
		t.Fatalf("empty pattern: want 400, got %d", code)
	}
	if code, _ := doJSON(t, http.MethodPut, srv.URL()+"/api/tag-rules/5", `{"tag_id":77,"app_pattern":"x"}`); code != 400 {
		t.Fatalf("unknown tag: want 400, got %d", code)
	}
	if code, _ := doJSON(t, http.MethodPut, srv.URL()+"/api/tag-rules/404", `{"tag_id":1,"app_pattern":"x"}`); code != 404 {
		t.Fatalf("missing rule: want 404, got %d", code)
	}
}

func TestAPI_AutoSubmit_GetDefaultsOff(t *testing.T) {
	db := setupAPIDB(t)
	srv := startTestServer(t, db)

	resp, err := http.Get(srv.URL() + "/api/auto-submit")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got map[string]any
	json.NewDecoder(resp.Body).Decode(&got)
	if got["enabled"] != false || got["interval_hours"] != float64(0) {
		t.Fatalf("want off by default, got %v", got)
	}
	if ids, ok := got["tag_ids"].([]any); !ok || len(ids) != 0 {
		t.Fatalf("tag_ids should be an empty array, got %v", got["tag_ids"])
	}
}

func TestAPI_AutoSubmit_SaveAndRead(t *testing.T) {
	db := setupAPIDB(t)
	srv := startTestServer(t, db)

	code, raw := doJSON(t, http.MethodPut, srv.URL()+"/api/auto-submit", `{"interval_hours":8,"tag_ids":[1]}`)
	if code != 200 {
		t.Fatalf("want 200, got %d: %s", code, raw)
	}
	var got map[string]any
	json.Unmarshal(raw, &got)
	if got["enabled"] != true || got["interval_hours"] != float64(8) {
		t.Fatalf("saved settings: %v", got)
	}
	if got["next_run"] == nil || got["enabled_at"] == nil {
		t.Fatalf("expected next_run and enabled_at when on: %v", got)
	}
	if ids := got["tag_ids"].([]any); len(ids) != 1 || ids[0] != float64(1) {
		t.Fatalf("tag_ids: %v", got["tag_ids"])
	}
}

func TestAPI_AutoSubmit_Validation(t *testing.T) {
	db := setupAPIDB(t)
	srv := startTestServer(t, db)

	if code, _ := doJSON(t, http.MethodPut, srv.URL()+"/api/auto-submit", `{"interval_hours":5,"tag_ids":[1]}`); code != 400 {
		t.Fatalf("unoffered interval: want 400, got %d", code)
	}
	if code, _ := doJSON(t, http.MethodPut, srv.URL()+"/api/auto-submit", `{"interval_hours":4,"tag_ids":[42]}`); code != 400 {
		t.Fatalf("unknown tag: want 400, got %d", code)
	}
}
