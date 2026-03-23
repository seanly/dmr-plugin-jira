// dmr-plugin-jira is a DMR external plugin exposing Jira REST API v2 tools (worklog, issue get, JQL search).
package main

import (
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/seanly/dmr/pkg/plugin/proto"
)

func main() {
	impl := NewJiraPlugin()
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: proto.Handshake,
		Plugins: map[string]goplugin.Plugin{
			"dmr-plugin": &proto.DMRPlugin{Impl: impl},
		},
	})
}
