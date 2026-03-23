# dmr-plugin-jira

DMR 外部插件：通过 [HashiCorp go-plugin](https://github.com/hashicorp/go-plugin) 向宿主进程提供 **Jira Server / Data Center REST API v2** 工具（登记工时、查询 issue、按项目搜索 issues）。

## 环境与依赖

| 项 | 说明 |
|----|------|
| **Jira** | 已在 **Jira Software 7.13.x**（例如 **7.13.2**）上，使用 **`/rest/api/2/...`** 验证：worklog、`/search`、issue `fields=timetracking`。其他 **Server / Data Center** 版本通常兼容；**Jira Cloud** 的 URL、版本路径或字段可能与本文不一致，需自行验证。 |
| **REST** | **API v2**（`rest/api/2`）。 |
| **认证** | **HTTP Basic**（用户名 + 密码，或贵司实例允许的 **API token** 作为密码）。 |
| **DMR** | 与主仓库 `github.com/seanly/dmr` 的 `pkg/plugin/proto` 协议一致；本地开发用 `replace` 指向 sibling `../dmr`（见 `go.mod`）。 |

## 构建与安装

```bash
make build    # 编译当前平台
make install  # 安装到 ~/.dmr/plugins/dmr-plugin-jira
```

交叉编译：`make cross-build`。

## 配置

在 DMR 主配置（如 `~/.dmr/config.yaml`）中增加插件项；`plugins[].config` 会序列化为 JSON 传给插件 `Init`（字段名与下表一致）。

完整示例片段见 [examples/dmr-config-snippet.yaml](examples/dmr-config-snippet.yaml)。

| 配置键 | 说明 |
|--------|------|
| `jira_url` | Jira 根 URL，无尾部斜杠，如 `https://issues.example.com`（若部署在 context path 需一并写上）。 |
| `user` | Jira 用户名。 |
| `password` | 密码或实例要求的 token（作 Basic 密码）。**勿提交到 git**，仅本机或密钥管理。 |
| `search_default_max_results` | 可选。`jiraIssuesSearch` 未传 `maxResults` 时返回条数；`0` 表示内置默认 **10**。 |
| `search_max_results_cap` | 可选。单次搜索 `maxResults` 上限；`0` 表示内置 **20**。若配置的上限小于默认条数，默认条数会降为该上限。 |

## 提供的 Tools

| Tool | 说明 |
|------|------|
| `jiraWorklogAdd` | `POST /rest/api/2/issue/{issueKey}/worklog`：参数 `issueKey`, `timeSpent`, `started`, `comment`。 |
| `jiraIssueGet` | `GET /rest/api/2/issue/{issueKey}`：可选 `fields`（逗号分隔）；默认 `summary,issuetype,status,assignee`。可加上 `timetracking` 查看剩余估算等。 |
| `jiraIssuesSearch` | `POST /rest/api/2/search`：必填 `projectKey`；可选 `issueType`、`maxResults`、`startAt`、`includeTimetracking`。**默认条数与上限**由配置 `search_default_max_results` / `search_max_results_cap` 控制（未配置时分别为 **10** / **20**）。内部使用安全拼装的 JQL（`projectKey` 校验为 Jira key 模式；`issueType` 经 JQL 转义）。 |

## OPA

默认策略中 **`jiraWorklogAdd`** 会触发 **require_approval**（写入外部系统）。只读工具 **`jiraIssueGet`**、**`jiraIssuesSearch`** 走默认 allow。可在宿主 `plugins/opapolicy/policies/` 中按需调整。

## 架构

```
DMR host ── go-plugin ──▶ dmr-plugin-jira
                              ├── jiraWorklogAdd
                              ├── jiraIssueGet
                              └── jiraIssuesSearch
                                      │
                                      ▼
                              Jira REST API v2
```

无 Webhook、无反向 RPC 触发宿主 `RunAgent`（与 `dmr-plugin-gitlab` 的 MR 审查流程不同）。

## 手动验证（curl）

参见你方已验证的 Basic Auth、`POST worklog` 与 `POST search`；插件发出的请求体与之一致。
