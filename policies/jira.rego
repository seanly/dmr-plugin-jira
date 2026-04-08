# OPA Policy for Jira plugin
# Rego V1 format

package dmr

# Allow: Read-only operations
decision := {"action": "allow", "reason": "jira read-only query", "risk": "low"} if {
	input.tool in [
		"jiraIssueGet",
		"jiraIssuesSearch",
		"jiraIssueWorklogs"
	]
}

# Require approval: Write operations
decision := {"action": "require_approval", "reason": "jira write: add worklog", "risk": "medium"} if {
	input.tool == "jiraWorklogAdd"
}
