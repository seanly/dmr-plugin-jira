package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var jiraProjectKeyRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

// JiraClient calls Jira Server/Data Center REST API v2.
type JiraClient struct {
	baseURL    string
	user       string
	password   string
	httpClient *http.Client
}

func NewJiraClient(baseURL, user, password string) *JiraClient {
	u := strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	return &JiraClient{
		baseURL:  u,
		user:     user,
		password: password,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c *JiraClient) doJSON(method, path string, query url.Values, body any) ([]byte, int, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		rdr = bytes.NewReader(b)
	}
	u := c.baseURL + path
	if query != nil && len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequest(method, u, rdr)
	if err != nil {
		return nil, 0, err
	}
	req.SetBasicAuth(c.user, c.password)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return data, resp.StatusCode, nil
}

// AddWorklog POST /rest/api/2/issue/{key}/worklog
func (c *JiraClient) AddWorklog(issueKey, timeSpent, started, comment string) (json.RawMessage, error) {
	key := strings.TrimSpace(issueKey)
	if key == "" {
		return nil, fmt.Errorf("issueKey is required")
	}
	payload := map[string]any{
		"timeSpent": timeSpent,
		"started":   started,
		"comment":   comment,
	}
	data, code, err := c.doJSON(http.MethodPost, "/rest/api/2/issue/"+url.PathEscape(key)+"/worklog", nil, payload)
	if err != nil {
		return nil, err
	}
	if code < 200 || code >= 300 {
		return nil, fmt.Errorf("jira worklog: HTTP %d: %s", code, truncate(string(data), 2000))
	}
	return json.RawMessage(data), nil
}

// GetIssue GET /rest/api/2/issue/{key}
func (c *JiraClient) GetIssue(issueKey, fields string) (json.RawMessage, error) {
	key := strings.TrimSpace(issueKey)
	if key == "" {
		return nil, fmt.Errorf("issueKey is required")
	}
	f := strings.TrimSpace(fields)
	if f == "" {
		f = "summary,issuetype,status,assignee"
	}
	q := url.Values{}
	q.Set("fields", f)
	data, code, err := c.doJSON(http.MethodGet, "/rest/api/2/issue/"+url.PathEscape(key), q, nil)
	if err != nil {
		return nil, err
	}
	if code < 200 || code >= 300 {
		return nil, fmt.Errorf("jira get issue: HTTP %d: %s", code, truncate(string(data), 2000))
	}
	return json.RawMessage(data), nil
}

// Search POST /rest/api/2/search
func (c *JiraClient) Search(jql string, fields []string, maxResults, startAt int) (json.RawMessage, error) {
	if strings.TrimSpace(jql) == "" {
		return nil, fmt.Errorf("jql is empty")
	}
	payload := map[string]any{
		"jql":        jql,
		"startAt":    startAt,
		"maxResults": maxResults,
		"fields":     fields,
	}
	data, code, err := c.doJSON(http.MethodPost, "/rest/api/2/search", nil, payload)
	if err != nil {
		return nil, err
	}
	if code < 200 || code >= 300 {
		return nil, fmt.Errorf("jira search: HTTP %d: %s", code, truncate(string(data), 2000))
	}
	return json.RawMessage(data), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func validateProjectKey(key string) error {
	k := strings.TrimSpace(key)
	if k == "" {
		return fmt.Errorf("projectKey is required")
	}
	if !jiraProjectKeyRe.MatchString(k) {
		return fmt.Errorf("invalid projectKey (expected Jira project key pattern)")
	}
	return nil
}

// escapeJQLString wraps a value in double quotes for JQL and escapes \ and ".
func escapeJQLString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func buildIssuesSearchJQL(projectKey, issueType string) (string, error) {
	if err := validateProjectKey(projectKey); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("project = ")
	b.WriteString(strings.TrimSpace(projectKey))
	it := strings.TrimSpace(issueType)
	if it != "" {
		b.WriteString(" AND issuetype = ")
		b.WriteString(escapeJQLString(it))
	}
	b.WriteString(" ORDER BY updated DESC")
	return b.String(), nil
}
