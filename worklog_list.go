package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	defaultWorklogMaxResults = 20
	maxWorklogMaxResultsCap  = 100
)

// jiraTimeLayouts tries to parse Jira worklog "started" timestamps.
func parseJiraStarted(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty started")
	}
	// Jira 7: 2026-03-23T09:00:00.000+0800 → insert colon in offset for Go Parse
	if m := jiraOffsetRE.FindStringSubmatch(s); len(m) == 5 {
		norm := m[1] + m[2] + m[3] + ":" + m[4]
		if t, err := time.Parse("2006-01-02T15:04:05.000-07:00", norm); err == nil {
			return t, nil
		}
	}
	return time.Parse(time.RFC3339, s)
}

var jiraOffsetRE = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3})([+-])(\d{2})(\d{2})$`)

// IssueWorklogsResult is returned to the LLM (compact worklogs, less noise than raw Jira JSON).
type IssueWorklogsResult struct {
	StartAt        int              `json:"startAt"`
	MaxResults     int              `json:"maxResults"`
	Total          int              `json:"total"`
	FilteredCount  int              `json:"filteredCount"`
	Worklogs       []compactWorklog `json:"worklogs"`
	Note           string           `json:"note,omitempty"`
}

type compactWorklog struct {
	ID               string         `json:"id"`
	Started          string         `json:"started"`
	TimeSpent        string         `json:"timeSpent,omitempty"`
	TimeSpentSeconds float64        `json:"timeSpentSeconds,omitempty"`
	Comment          string         `json:"comment,omitempty"`
	Author           compactAuthor  `json:"author"`
}

type compactAuthor struct {
	Name        string `json:"name,omitempty"`
	Key         string `json:"key,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
}

type rawWorklogPage struct {
	StartAt    int                      `json:"startAt"`
	MaxResults int                      `json:"maxResults"`
	Total      int                      `json:"total"`
	Worklogs   []map[string]interface{} `json:"worklogs"`
}

// GetIssueWorklogs fetches one page from Jira, optionally filters, returns compact JSON-friendly struct.
func (c *JiraClient) GetIssueWorklogs(issueKey string, startAt, maxResults int, authorName, startedFrom, startedTo string) (*IssueWorklogsResult, error) {
	key := strings.TrimSpace(issueKey)
	if key == "" {
		return nil, fmt.Errorf("issueKey is required")
	}
	if maxResults <= 0 {
		maxResults = defaultWorklogMaxResults
	}
	if maxResults > maxWorklogMaxResultsCap {
		maxResults = maxWorklogMaxResultsCap
	}
	if startAt < 0 {
		startAt = 0
	}
	q := url.Values{}
	q.Set("startAt", fmt.Sprintf("%d", startAt))
	q.Set("maxResults", fmt.Sprintf("%d", maxResults))

	data, code, err := c.doJSON(http.MethodGet, "/rest/api/2/issue/"+url.PathEscape(key)+"/worklog", q, nil)
	if err != nil {
		return nil, err
	}
	if code < 200 || code >= 300 {
		return nil, fmt.Errorf("jira list worklog: HTTP %d: %s", code, truncate(string(data), 2000))
	}

	var page rawWorklogPage
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	if err := dec.Decode(&page); err != nil {
		return nil, fmt.Errorf("parse worklog json: %w", err)
	}

	var fromT, toT *time.Time
	if s := strings.TrimSpace(startedFrom); s != "" {
		t, err := parseJiraStarted(s)
		if err != nil {
			return nil, fmt.Errorf("startedFrom: %w", err)
		}
		fromT = &t
	}
	if s := strings.TrimSpace(startedTo); s != "" {
		t, err := parseJiraStarted(s)
		if err != nil {
			return nil, fmt.Errorf("startedTo: %w", err)
		}
		toT = &t
	}
	authorWant := strings.TrimSpace(authorName)

	out := &IssueWorklogsResult{
		StartAt:    page.StartAt,
		MaxResults: page.MaxResults,
		Total:      page.Total,
		Note:       "total/startAt/maxResults are Jira pagination (unfiltered). worklogs are filtered then compacted (no avatar/email).",
	}

	for _, w := range page.Worklogs {
		startedStr := stringField(w, "started")
		if startedStr == "" {
			continue
		}
		st, err := parseJiraStarted(startedStr)
		if err != nil {
			continue
		}
		if fromT != nil && st.Before(*fromT) {
			continue
		}
		if toT != nil && st.After(*toT) {
			continue
		}
		auth := authorMap(w["author"])
		if authorWant != "" {
			if !strings.EqualFold(auth.Name, authorWant) && !strings.EqualFold(auth.Key, authorWant) {
				continue
			}
		}
		out.Worklogs = append(out.Worklogs, compactOneWorklog(w, auth))
	}
	out.FilteredCount = len(out.Worklogs)
	return out, nil
}

func authorMap(v interface{}) compactAuthor {
	m, ok := v.(map[string]interface{})
	if !ok {
		return compactAuthor{}
	}
	return compactAuthor{
		Name:        stringField(m, "name"),
		Key:         stringField(m, "key"),
		DisplayName: stringField(m, "displayName"),
	}
}

func compactOneWorklog(w map[string]interface{}, auth compactAuthor) compactWorklog {
	cw := compactWorklog{
		ID:        stringField(w, "id"),
		Started:   stringField(w, "started"),
		TimeSpent: stringField(w, "timeSpent"),
		Comment:   stringField(w, "comment"),
		Author:    auth,
	}
	if n, ok := w["timeSpentSeconds"]; ok {
		switch x := n.(type) {
		case json.Number:
			if f, err := x.Float64(); err == nil {
				cw.TimeSpentSeconds = f
			}
		case float64:
			cw.TimeSpentSeconds = x
		}
	}
	return cw
}

func stringField(m map[string]interface{}, k string) string {
	v, ok := m[k]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	default:
		return fmt.Sprint(t)
	}
}
