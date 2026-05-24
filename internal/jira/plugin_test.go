package jira

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/seanly/dmr/pkg/plugin/proto"
)

func TestJiraPlugin_CallTool_sprintIssues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/agile/1.0/sprint/5/issue" && r.Method == http.MethodGet {
			if r.URL.Query().Get("maxResults") != "10" || r.URL.Query().Get("startAt") != "0" {
				t.Errorf("query: %v", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"issues":[],"maxResults":10,"startAt":0,"total":0}`)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	cfg := map[string]string{
		"jira_url": srv.URL,
		"user":     "u",
		"password": "p",
	}
	cfgJSON, _ := json.Marshal(cfg)

	p := NewJiraPlugin()
	initReq := proto.InitRequest{ConfigJSON: string(cfgJSON)}
	initResp := proto.InitResponse{}
	if err := p.Init(&initReq, &initResp); err != nil {
		t.Fatal(err)
	}

	args, _ := json.Marshal(map[string]any{"sprintId": float64(5)})
	callReq := proto.CallToolRequest{Name: "jiraSprintIssues", ArgsJSON: string(args)}
	callResp := proto.CallToolResponse{}
	if err := p.CallTool(&callReq, &callResp); err != nil {
		t.Fatal(err)
	}
	if callResp.Error != "" {
		t.Fatal(callResp.Error)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(callResp.ResultJSON), &out); err != nil {
		t.Fatal(err)
	}
	if _, ok := out["issues"]; !ok {
		t.Fatalf("unexpected result: %s", callResp.ResultJSON)
	}
}
