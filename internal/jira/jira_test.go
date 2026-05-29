package jira

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildAssigneeIssuesJQL(t *testing.T) {
	jql, err := buildAssigneeIssuesJQL("INF", "jdoe", nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	want := `project = INF AND assignee = "jdoe" ORDER BY updated DESC`
	if jql != want {
		t.Fatalf("got %q want %q", jql, want)
	}

	jql2, err := buildAssigneeIssuesJQL("", "u_1@test", []string{"Story", " Task "}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	want2 := `assignee = "u_1@test" AND (issuetype = "Story" OR issuetype = "Task") ORDER BY updated DESC`
	if jql2 != want2 {
		t.Fatalf("got %q want %q", jql2, want2)
	}
}

func TestBuildEpicLinkedJQL(t *testing.T) {
	jql, err := buildEpicLinkedJQL("INF-771")
	if err != nil {
		t.Fatal(err)
	}
	want := `"Epic Link" = INF-771`
	if jql != want {
		t.Fatalf("got %q want %q", jql, want)
	}
}

func TestAssigneeIssuesSearch_HTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/2/search" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var m map[string]any
		if err := json.Unmarshal(body, &m); err != nil {
			t.Fatal(err)
		}
		jql := m["jql"].(string)
		want := `project = XYZ AND assignee = "alice" AND (issuetype = "Bug") ORDER BY updated DESC`
		if jql != want {
			t.Fatalf("jql:\n got  %s\nwant %s", jql, want)
		}
		_, _ = w.Write([]byte(`{"issues":[],"total":0}`))
	}))
	t.Cleanup(srv.Close)

	c := NewJiraClient(srv.URL, "u", "p")
	_, err := c.AssigneeIssuesSearch("XYZ", "alice", []string{"Bug"}, []string{"summary"}, 5, 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
}

func TestEpicLinkedIssuesSearch_HTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/2/search" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Fatal(err)
		}
		jql := decoded["jql"].(string)
		want := `"Epic Link" = DEMO-42 ORDER BY updated DESC`
		if jql != want {
			t.Fatalf("jql:\n got  %q\nwant %q", jql, want)
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	c := NewJiraClient(srv.URL, "x", "y")
	_, err := c.EpicLinkedIssuesSearch("DEMO-42", []string{"summary"}, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSprintIssues_HTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("want GET got %s", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/rest/agile/1.0/sprint/") ||
			r.URL.Path != "/rest/agile/1.0/sprint/99/issue" ||
			r.URL.Query().Get("startAt") != "3" ||
			r.URL.Query().Get("maxResults") != "7" {
			t.Fatalf("bad URL: %s %v", r.URL.Path, r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"issues":[],"total":0}`))
	}))
	t.Cleanup(srv.Close)
	c := NewJiraClient(srv.URL, "a", "b")
	_, err := c.SprintIssues(99, 3, 7)
	if err != nil {
		t.Fatal(err)
	}
}
