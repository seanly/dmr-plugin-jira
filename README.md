# dmr-plugin-jira

DMR 外部插件：通过 [HashiCorp go-plugin](https://github.com/hashicorp/go-plugin) 向宿主进程提供 **Jira Server / Data Center REST API v2** 工具（登记工时、查询 issue、按项目搜索 issues）。

## 环境与依赖

| 项 | 说明 |
|----|------|
| **Jira** | 已在 **Jira Software 7.13.x**（例如 **7.13.2**）上，使用 **`/rest/api/2/...`** 验证：worklog、`/search`、issue `fields=timetracking`。其他 **Server / Data Center** 版本通常兼容；**Jira Cloud** 的 URL、版本路径或字段可能与本文不一致，需自行验证。 |
| **REST** | **API v2**（`rest/api/2`）；迭代相关只读 **`/rest/agile/1.0`**（Greenhopper / Jira Agile）。 |
| **认证** | **HTTP Basic**（用户名 + 密码，或贵司实例允许的 **API token** 作为密码）。 |
| **DMR** | 与 `github.com/seanly/dmr` 的 `pkg/plugin/proto` 一致；本地开发与 sibling `../dmr` 使用 **`go.work`**（复制 `go.work.example`），或使用 `make bump-dmr` 更新 `go.mod` 中的 pseudo-version。 |

## 构建与安装

目录结构：**`cmd/dmr-plugin-jira`**（入口）+ **`internal/jira`**（实现）。产物输出到 **`bin/`**。

```bash
cp go.work.example go.work    # 按需调整 ../dmr 路径
make build                      # 等价: go build -o bin/dmr-plugin-jira ./cmd/dmr-plugin-jira
make install                    # 复制到 ~/.dmr/plugins/
make install-policy             # jira.rego → ~/.dmr/etc/policies/（目录 0700、文件 0600，兼容 DMR opa 热加载校验）
make install-all
make help
```

按需将 `github.com/seanly/dmr` 对齐到本地 `dmr`：

```bash
make bump-dmr
# make bump-dmr DMR_DIR=/path/to/dmr
```

交叉编译：`make cross-build`（输出在 `bin/`）。

## 开发与测试

```bash
go test ./...
```

DMR 宿主主配置为 **TOML**（如 `~/.dmr/config.toml`，或通过 `dmr ... --config path/to/config.toml` 指定）。在 `[[plugins]]` 中注册本插件后，`[plugins.config]` 会以 **JSON** 序列化传给插件 `Init`（下表字段名与 JSON 键一致，在 TOML 中仍用 **snake_case** 书写）。

示例：

```toml
[[plugins]]
name = "jira"
enabled = true
# 使用绝对路径，例如 ~/.dmr/plugins/dmr-plugin-jira（以 make install 目标为准）
path = "/absolute/path/to/dmr-plugin-jira"

[plugins.config]
jira_url = "https://issues.example.com"
user = "svc-dmr"
password = "YOUR_TOKEN_OR_PASSWORD"               # 勿提交到 git
# 可选：搜索默认条数与单次上限
# search_default_max_results = 10
# search_max_results_cap = 20
```

载入本仓库策略时，在 **`opa_policy`** 的 `policies` 列表中加入 `jira.rego` 的路径即可（与 Jenkins 等插件相同，见 DMR `AGENTS.md`）。

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
| `jiraAssigneeIssues` | `POST /rest/api/2/search`：必填 `assignee`（用户名风格 token，`assignee = "..."`）；可选 `projectKey`（省略则不按项目筛选）、`issueTypes`（至多 **10** 个类型名，OR）；`maxResults` / `startAt` / `includeTimetracking` 与 `jiraIssuesSearch` 相同 cap。返回字段含 `updated`，按更新时间倒序。 |
| `jiraEpicLinkedIssues` | `POST /rest/api/2/search`：必填 `epicKey`（如 `INF-123`）；JQL 使用 classic Scrum 的 **`"Epic Link" = epicKey`**（字段名不符的实例会返回 Jira 错误）。同上 cap / `includeTimetracking`；返回按 `updated` 倒序。 |
| `jiraSprintIssues` | `GET /rest/agile/1.0/sprint/{sprintId}/issue`：**`sprintId` 为数字 ID**（非名称）。`startAt`、`maxResults` 使用与搜索相同的默认与上限。响应为 Agile API 原生 JSON（含 `issues`、`total` 等，视版本而定）。 |
| `jiraIssueWorklogs` | `GET /rest/api/2/issue/{key}/worklog`：分页 `startAt` / `maxResults`（默认 **20**，上限 **100**）。可选 `authorName`、`startedFrom`、`startedTo`（与 Jira `started` 同形，如 `...000+0800`）。**返回体已精简**：每条只含 `id`、`started`、`timeSpent`、`timeSpentSeconds`、`comment`、`author`（仅 `name`/`key`/`displayName`），去掉 `avatarUrls`、`emailAddress`、`self` 等大字段，减少上下文体积。`total` 为 Jira 侧未过滤的总数；过滤只作用于当前页，翻页需递增 `startAt`。 |

## OPA

默认策略中 **`jiraWorklogAdd`** 会触发 **require_approval**（写入外部系统）。只读工具 **`jiraIssueGet`**、**`jiraIssuesSearch`**、**`jiraAssigneeIssues`**、**`jiraEpicLinkedIssues`**、**`jiraSprintIssues`**、**`jiraIssueWorklogs`** 走默认 allow。可在宿主 `plugins/opapolicy/policies/` 中按需调整。

## 架构

```
DMR host ── go-plugin ──▶ dmr-plugin-jira (cmd/dmr-plugin-jira → internal/jira)
                              ├── jiraWorklogAdd
                              ├── jiraIssueGet
                              ├── jiraIssuesSearch / jiraAssigneeIssues / jiraEpicLinkedIssues / jiraSprintIssues
                              └── jiraIssueWorklogs
                                      │
                                      ▼
                              Jira REST API v2 + Agile REST (sprint issues)
```

无 Webhook、无反向 RPC 触发宿主 `RunAgent`（与 `dmr-plugin-gitlab` 的 MR 审查流程不同）。

宿主与 DMR **`pkg/plugin/proto` 对齐**后，插件可实现 **`ProvideSystemPrompt`**：`Fragment` 会并入 DMR 与各内置插件相同的 **system prompt** 拼装链（不暴露密码：`user` 仅作说明）。此处用于写明 **`user` 是 REST 调用身份**，**不等价**于聊天用户；可按需结合 `RunAgent` 的 ContextJSON。

## jiraIssueWorklogs（原子查询 + 条件）

| 参数 | 说明 |
|------|------|
| `issueKey` | 必填。 |
| `startAt` | 可选，默认 `0`，原样传给 Jira 分页。 |
| `maxResults` | 可选，默认 **20**，上限 **100**。 |
| `authorName` | 可选；只保留 `author.name` 或 `author.key`（不区分大小写）。 |
| `startedFrom` / `startedTo` | 可选；与 Jira `started` 同形（如 `2026-03-10T00:00:00.000+0800`）；含边界（`started >= startedFrom` 且 `started <= startedTo`）。 |

**语义（重要）**

- Jira 的 **`total` / `startAt` / `maxResults`** 表示**未按 author/时间过滤**的分页；`worklogs` 数组是**本页拉取后再过滤、再精简字段**后的结果。
- **`filteredCount`** = 当前返回的 `worklogs` 条数。过滤**只作用于当前页**；若本页过滤后很少，仍可能有其它页满足条件，需 **`startAt += maxResults`**（或按 Jira 约定递增）多次调用，直到 `startAt >= total`。
- 返回体**已去掉** `avatarUrls`、`emailAddress`、`self` 等，仅保留必要字段，减小上下文。

## 常见问题（网络）

若日志出现 **`connection reset by peer`**、**`EOF`**（例如在 `POST .../rest/api/2/search` 途中），多数是 **链路或中间设备**（SSL 检测、网关、防火墙、VPN、代理）把连接断开，而不是 Jira 返回的业务 HTTP JSON 错误。对端若为 **`198.18.0.0/15`** 等隧道/劫持常见段，可先检查出口路由。插件对上述传输层 **`transient`** 类错误会自动 **最多重试 3 次**（间隔约 300ms、600ms）；仍失败时请在与 DMR **相同出口**上用 `curl` 复现并排查基建。仅用 `assignee`、不带 **`projectKey`** 的搜索范围大、易被网关限流——可先加 **`projectKey`** 收窄 JQL。

## 手动验证（curl）

列出某 issue 的 worklog（原始 Jira JSON，用于对照字段）：

```bash
curl -sS -u "$JIRA_USER:$JIRA_PASS" \
  "$JIRA_BASE/rest/api/2/issue/$ISSUE_KEY/worklog?startAt=0&maxResults=50"
```

响应中应含 `startAt`、`maxResults`、`total`、`worklogs`；每条含 `started`、`author`、`timeSpentSeconds` 等。插件 `jiraIssueWorklogs` 使用同一 GET，再压缩与可选过滤。

其它：Basic Auth、`POST worklog`、`POST search` 与插件一致。
