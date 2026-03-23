package main

// JiraPluginConfig is loaded from DMR plugins[].config (JSON keys match struct tags).
type JiraPluginConfig struct {
	JiraURL  string `json:"jira_url"`
	User     string `json:"user"`
	Password string `json:"password"` // password or API token, per Jira instance policy

	// SearchDefaultMaxResults: jiraIssuesSearch 未传 maxResults 时的条数。0 表示用内置默认（10）。
	SearchDefaultMaxResults int `json:"search_default_max_results"`
	// SearchMaxResultsCap: jiraIssuesSearch 的 maxResults 上限。0 表示用内置上限（20）。
	SearchMaxResultsCap int `json:"search_max_results_cap"`
}

func defaultConfig() JiraPluginConfig {
	return JiraPluginConfig{}
}
