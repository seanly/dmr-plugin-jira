package jira

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const transientHTTPRetries = 3 // total attempts including the first request

var (
	jiraProjectKeyRe   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
	jiraAssigneeRe     = regexp.MustCompile(`^[a-zA-Z0-9._@+-]+$`)
	jiraEpicIssueKeyRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]*-\d+$`)
)

const maxIssueTypesInJQLFilter = 10

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
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
	}

	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	for attempt := 1; attempt <= transientHTTPRetries; attempt++ {
		var rdr io.Reader
		if len(payload) > 0 {
			rdr = bytes.NewReader(payload)
		}
		req, err := http.NewRequest(method, u, rdr)
		if err != nil {
			return nil, 0, err
		}
		req.SetBasicAuth(c.user, c.password)
		if len(payload) > 0 {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if attempt < transientHTTPRetries && transientTransportErr(err) {
				time.Sleep(time.Duration(attempt) * 300 * time.Millisecond)
				continue
			}
			return nil, 0, err
		}

		data, readErr := io.ReadAll(resp.Body)
		code := resp.StatusCode
		resp.Body.Close()
		if readErr != nil {
			if attempt < transientHTTPRetries && transientTransportErr(readErr) {
				time.Sleep(time.Duration(attempt) * 300 * time.Millisecond)
				continue
			}
			return nil, code, readErr
		}
		return data, code, nil
	}
	return nil, 0, fmt.Errorf("BUG: unreachable: transientHTTPRetries=%d", transientHTTPRetries)
}

// transientTransportErr is true when the failure is commonly caused by flaky networks /
// proxies (safe to retry the same REST call).
func transientTransportErr(err error) bool {
	if err == nil {
		return false
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}

	for e := err; e != nil; e = errors.Unwrap(e) {
		if errors.Is(e, io.EOF) || errors.Is(e, io.ErrUnexpectedEOF) {
			return true
		}
	}

	s := strings.ToLower(err.Error())
	return strings.Contains(s, "connection reset") ||
		strings.Contains(s, "broken pipe") ||
		strings.Contains(s, "connection refused") ||
		strings.Contains(s, " eof") ||
		strings.HasSuffix(s, ": eof")
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

func validateJQLDate(s string) error {
	if matched := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`).MatchString(strings.TrimSpace(s)); !matched {
		return fmt.Errorf("invalid date format, expected yyyy-MM-dd")
	}
	return nil
}

func buildIssuesSearchJQL(projectKey, issueType, updatedFrom, updatedTo string) (string, error) {
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
	if s := strings.TrimSpace(updatedFrom); s != "" {
		if err := validateJQLDate(s); err != nil {
			return "", fmt.Errorf("updatedFrom: %w", err)
		}
		b.WriteString(" AND updated >= ")
		b.WriteString(escapeJQLString(s))
	}
	if s := strings.TrimSpace(updatedTo); s != "" {
		if err := validateJQLDate(s); err != nil {
			return "", fmt.Errorf("updatedTo: %w", err)
		}
		b.WriteString(" AND updated <= ")
		b.WriteString(escapeJQLString(s))
	}
	b.WriteString(" ORDER BY updated DESC")
	return b.String(), nil
}

func validateAssigneeToken(assignee string) error {
	a := strings.TrimSpace(assignee)
	if a == "" {
		return fmt.Errorf("assignee is required")
	}
	if len(a) > 256 || !jiraAssigneeRe.MatchString(a) {
		return fmt.Errorf("invalid assignee (expected Jira username-style token)")
	}
	return nil
}

// capIssueTypeNames trims, drops empties, and keeps at most maxIssueTypesInJQLFilter names.
func capIssueTypeNames(issueTypes []string) []string {
	var out []string
	for _, it := range issueTypes {
		t := strings.TrimSpace(it)
		if t == "" {
			continue
		}
		out = append(out, t)
		if len(out) >= maxIssueTypesInJQLFilter {
			break
		}
	}
	return out
}

// buildAssigneeIssuesJQL builds JQL for issues assigned to a user, ordered by updated DESC.
// projectKey optional: when empty, no project clause (respects user's browse permissions).
// issueTypes optional OR filter on issuetype (capped).
func buildAssigneeIssuesJQL(projectKey, assignee string, issueTypes []string, updatedFrom, updatedTo string) (string, error) {
	if err := validateAssigneeToken(assignee); err != nil {
		return "", err
	}
	var b strings.Builder
	pk := strings.TrimSpace(projectKey)
	if pk != "" {
		if err := validateProjectKey(pk); err != nil {
			return "", err
		}
		b.WriteString("project = ")
		b.WriteString(pk)
		b.WriteString(" AND ")
	}
	b.WriteString("assignee = ")
	b.WriteString(escapeJQLString(strings.TrimSpace(assignee)))

	types := capIssueTypeNames(issueTypes)
	if len(types) > 0 {
		b.WriteString(" AND (")
		for i, it := range types {
			if i > 0 {
				b.WriteString(" OR ")
			}
			b.WriteString("issuetype = ")
			b.WriteString(escapeJQLString(it))
		}
		b.WriteString(")")
	}
	if s := strings.TrimSpace(updatedFrom); s != "" {
		if err := validateJQLDate(s); err != nil {
			return "", fmt.Errorf("updatedFrom: %w", err)
		}
		b.WriteString(" AND updated >= ")
		b.WriteString(escapeJQLString(s))
	}
	if s := strings.TrimSpace(updatedTo); s != "" {
		if err := validateJQLDate(s); err != nil {
			return "", fmt.Errorf("updatedTo: %w", err)
		}
		b.WriteString(" AND updated <= ")
		b.WriteString(escapeJQLString(s))
	}
	b.WriteString(" ORDER BY updated DESC")
	return b.String(), nil
}

func buildEpicLinkedJQL(epicKey string) (string, error) {
	k := strings.TrimSpace(epicKey)
	if k == "" {
		return "", fmt.Errorf("epicKey is required")
	}
	if !jiraEpicIssueKeyRe.MatchString(k) {
		return "", fmt.Errorf("invalid epicKey (expected issue key pattern e.g. PROJ-123)")
	}
	var b strings.Builder
	b.WriteString(`"Epic Link" = `)
	b.WriteString(k)
	return b.String(), nil
}

// AssigneeIssuesSearch POST /rest/api/2/search with constrained assignee (+ optional project / types / updated range) JQL.
func (c *JiraClient) AssigneeIssuesSearch(projectKey, assignee string, issueTypes []string, fields []string, maxResults, startAt int, updatedFrom, updatedTo string) (json.RawMessage, error) {
	jql, err := buildAssigneeIssuesJQL(projectKey, assignee, issueTypes, updatedFrom, updatedTo)
	if err != nil {
		return nil, err
	}
	return c.Search(jql, fields, maxResults, startAt)
}

// EpicLinkedIssuesSearch POST /rest/api/2/search with classic Scrum "Epic Link" semantics.
func (c *JiraClient) EpicLinkedIssuesSearch(epicKey string, fields []string, maxResults, startAt int) (json.RawMessage, error) {
	jql, err := buildEpicLinkedJQL(epicKey)
	if err != nil {
		return nil, err
	}
	order := ` ORDER BY updated DESC`
	combined := jql + order
	return c.Search(combined, fields, maxResults, startAt)
}

// SprintIssues GET /rest/agile/1.0/sprint/{id}/issue
func (c *JiraClient) SprintIssues(sprintID int64, startAt, maxResults int) (json.RawMessage, error) {
	if sprintID <= 0 {
		return nil, fmt.Errorf("sprintId must be a positive integer")
	}
	if startAt < 0 {
		startAt = 0
	}
	q := url.Values{}
	q.Set("startAt", strconv.Itoa(startAt))
	q.Set("maxResults", strconv.Itoa(maxResults))
	path := "/rest/agile/1.0/sprint/" + strconv.FormatInt(sprintID, 10) + "/issue"
	data, code, err := c.doJSON(http.MethodGet, path, q, nil)
	if err != nil {
		return nil, err
	}
	if code < 200 || code >= 300 {
		return nil, fmt.Errorf("jira agile sprint issues: HTTP %d: %s", code, truncate(string(data), 2000))
	}
	return json.RawMessage(data), nil
}
