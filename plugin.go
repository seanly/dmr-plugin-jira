package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/seanly/dmr/pkg/plugin/proto"
)

const (
	defaultSearchMaxResults = 10
	maxSearchMaxResultsCap  = 20
)

// JiraPlugin implements proto.DMRPluginInterface for Jira REST tools.
type JiraPlugin struct {
	config JiraPluginConfig
	client *JiraClient

	// Effective search limits after Init (config or built-in defaults, normalized).
	searchDefaultMax int
	searchMaxCap     int
}

func NewJiraPlugin() *JiraPlugin {
	return &JiraPlugin{config: defaultConfig()}
}

func (p *JiraPlugin) Init(req *proto.InitRequest, resp *proto.InitResponse) error {
	if req.ConfigJSON != "" {
		if err := json.Unmarshal([]byte(req.ConfigJSON), &p.config); err != nil {
			return fmt.Errorf("parse config: %w", err)
		}
	}
	if strings.TrimSpace(p.config.JiraURL) == "" || strings.TrimSpace(p.config.User) == "" {
		return fmt.Errorf("jira_url and user are required")
	}
	if p.config.Password == "" {
		return fmt.Errorf("password is required (password or token per your Jira instance)")
	}
	def := p.config.SearchDefaultMaxResults
	capN := p.config.SearchMaxResultsCap
	if def <= 0 {
		def = defaultSearchMaxResults
	}
	if capN <= 0 {
		capN = maxSearchMaxResultsCap
	}
	if capN < def {
		def = capN
	}
	p.searchDefaultMax = def
	p.searchMaxCap = capN

	p.client = NewJiraClient(p.config.JiraURL, p.config.User, p.config.Password)
	log.Printf("dmr-plugin-jira: initialized for %s (issues search default=%d cap=%d)", p.config.JiraURL, p.searchDefaultMax, p.searchMaxCap)
	return nil
}

func (p *JiraPlugin) Shutdown(req *proto.ShutdownRequest, resp *proto.ShutdownResponse) error {
	return nil
}

func (p *JiraPlugin) RequestApproval(req *proto.ApprovalRequest, resp *proto.ApprovalResult) error {
	resp.Choice = 0
	resp.Comment = "jira plugin does not handle approvals"
	return nil
}

func (p *JiraPlugin) RequestBatchApproval(req *proto.BatchApprovalRequest, resp *proto.BatchApprovalResult) error {
	resp.Choice = 0
	return nil
}

func (p *JiraPlugin) ProvideTools(req *proto.ProvideToolsRequest, resp *proto.ProvideToolsResponse) error {
	resp.Tools = []proto.ToolDef{
		{
			Name:           "jiraWorklogAdd",
			Description:    "在 Jira issue（含 Epic）上登记标准工作日志（REST worklog）",
			ParametersJSON: `{"type": "object", "properties": {"issueKey": {"type": "string", "description": "Issue key，如 INF-771"}, "timeSpent": {"type": "string", "description": "耗时，Jira 时长表达式，如 8h、30m、1m"}, "started": {"type": "string", "description": "开始时间，含时区，如 2026-03-23T09:00:00.000+0800"}, "comment": {"type": "string", "description": "工作说明"}}, "required": ["issueKey", "timeSpent", "started", "comment"]}`,
			Group:          "extended",
			SearchHint:     "jira, worklog, log, time, track, 工时, 登记, 记录",
		},
		{
			Name:           "jiraIssueGet",
			Description:    "获取单个 Jira issue 的字段（可含 timetracking 剩余估算等）",
			ParametersJSON: `{"type": "object", "properties": {"issueKey": {"type": "string", "description": "Issue key"}, "fields": {"type": "string", "description": "逗号分隔字段名；留空则默认 summary,issuetype,status,assignee"}}, "required": ["issueKey"]}`,
			Group:          "extended",
			SearchHint:     "jira, issue, get, detail, info, 问题, 获取, 详情",
		},
		{
			Name:           "jiraIssuesSearch",
			Description:    "按项目 key 搜索 issues，可选 issuetype 过滤；可选包含 timetracking",
			ParametersJSON: `{"type": "object", "properties": {"projectKey": {"type": "string", "description": "项目 key，如 INF"}, "issueType": {"type": "string", "description": "可选，类型名如 Epic；不传则不限定类型"}, "maxResults": {"type": "integer", "description": "默认与上限见宿主 config 中 search_default_max_results / search_max_results_cap；未配置时默认 10、上限 20"}, "startAt": {"type": "integer", "description": "分页偏移，默认 0"}, "includeTimetracking": {"type": "boolean", "description": "为 true 时在结果中包含 timetracking 字段"}}, "required": ["projectKey"]}`,
			Group:          "extended",
			SearchHint:     "jira, issue, search, find, project, epic, 问题, 搜索, 查找, 项目",
		},
		{
			Name:           "jiraIssueWorklogs",
			Description:    "分页列出某 issue 的工作日志；返回精简字段（无头像/邮箱）；可选按登记人、started 时间范围过滤。total 为 Jira 未过滤总数，需翻页时增大 startAt",
			ParametersJSON: `{"type": "object", "properties": {"issueKey": {"type": "string", "description": "Issue key"}, "startAt": {"type": "integer", "description": "分页，默认 0"}, "maxResults": {"type": "integer", "description": "每页条数，默认 20，最大 100"}, "authorName": {"type": "string", "description": "可选，只保留 author.name 或 author.key 匹配的条目"}, "startedFrom": {"type": "string", "description": "可选，started 下限，建议与 Jira 一致如 2026-03-10T00:00:00.000+0800"}, "startedTo": {"type": "string", "description": "可选，started 上限（含该时刻）"}}, "required": ["issueKey"]}`,
			Group:          "extended",
			SearchHint:     "jira, worklog, list, history, issue, 工时, 列表, 历史",
		},
	}
	return nil
}

func (p *JiraPlugin) CallTool(req *proto.CallToolRequest, resp *proto.CallToolResponse) error {
	var args map[string]any
	if err := json.Unmarshal([]byte(req.ArgsJSON), &args); err != nil {
		resp.Error = fmt.Sprintf("parse args: %v", err)
		return nil
	}
	result, err := p.executeTool(req.Name, args)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	out, _ := json.Marshal(result)
	resp.ResultJSON = string(out)
	return nil
}

func (p *JiraPlugin) executeTool(name string, args map[string]any) (any, error) {
	if p.client == nil {
		return nil, fmt.Errorf("jira client not initialized")
	}
	switch name {
	case "jiraWorklogAdd":
		issueKey, _ := args["issueKey"].(string)
		timeSpent, _ := args["timeSpent"].(string)
		started, _ := args["started"].(string)
		comment, _ := args["comment"].(string)
		return p.client.AddWorklog(issueKey, timeSpent, started, comment)
	case "jiraIssueGet":
		issueKey, _ := args["issueKey"].(string)
		fields, _ := args["fields"].(string)
		return p.client.GetIssue(issueKey, fields)
	case "jiraIssuesSearch":
		projectKey, _ := args["projectKey"].(string)
		issueType, _ := args["issueType"].(string)
		defMax := p.searchDefaultMax
		maxCap := p.searchMaxCap
		if defMax <= 0 {
			defMax = defaultSearchMaxResults
		}
		if maxCap <= 0 {
			maxCap = maxSearchMaxResultsCap
		}
		maxResults := intArg(args, "maxResults", defMax)
		if maxResults <= 0 {
			maxResults = defMax
		}
		if maxResults > maxCap {
			maxResults = maxCap
		}
		startAt := intArg(args, "startAt", 0)
		if startAt < 0 {
			startAt = 0
		}
		includeTT := boolArg(args, "includeTimetracking")
		jql, err := buildIssuesSearchJQL(projectKey, issueType)
		if err != nil {
			return nil, err
		}
		fields := []string{"summary", "issuetype", "status", "assignee", "updated"}
		if includeTT {
			fields = append(fields, "timetracking")
		}
		return p.client.Search(jql, fields, maxResults, startAt)
	case "jiraIssueWorklogs":
		issueKey, _ := args["issueKey"].(string)
		wStart := intArg(args, "startAt", 0)
		wMax := intArg(args, "maxResults", defaultWorklogMaxResults)
		authorName, _ := args["authorName"].(string)
		startedFrom, _ := args["startedFrom"].(string)
		startedTo, _ := args["startedTo"].(string)
		return p.client.GetIssueWorklogs(issueKey, wStart, wMax, authorName, startedFrom, startedTo)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

func intArg(args map[string]any, key string, def int) int {
	v, ok := args[key]
	if !ok || v == nil {
		return def
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	default:
		return def
	}
}

func boolArg(args map[string]any, key string) bool {
	v, ok := args[key]
	if !ok || v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}
