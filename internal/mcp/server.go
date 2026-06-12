// Package mcp is the mix point of the MCP plane: transport glue, tool
// registration, and request routing. Tool behavior lives in the
// per-tool packages (plan, check); product logic lives behind them.
//
// Hard boundary: the coding agent never talks directly to the backend,
// and the backend never talks directly to the coding agent. This MCP
// plane is the only bridge.
package mcp

import (
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Riad374-code/architectmcp/internal/mcp/remotecheck"
	"github.com/Riad374-code/architectmcp/internal/mcp/remoteplan"
)

const (
	serverName    = "architectmcp"
	serverVersion = "0.1.0"
)

// ServerOption configures hosted tool integrations.
type ServerOption func(*serverOptions)

type serverOptions struct {
	planExecutor  remoteplan.Executor
	checkExecutor remotecheck.Executor
}

// WithPlanExecutor routes architect_plan calls to backend business logic.
func WithPlanExecutor(executor remoteplan.Executor) ServerOption {
	return func(options *serverOptions) {
		options.planExecutor = executor
	}
}

// WithCheckExecutor routes architect_check calls to backend business logic.
func WithCheckExecutor(executor remotecheck.Executor) ServerOption {
	return func(options *serverOptions) {
		options.checkExecutor = executor
	}
}

// NewServer builds one stateless MCP server with hosted-ready tools registered.
func NewServer(options ...ServerOption) *sdk.Server {
	config := serverOptions{}
	for _, option := range options {
		if option != nil {
			option(&config)
		}
	}
	server := sdk.NewServer(&sdk.Implementation{
		Name:    serverName,
		Version: serverVersion,
	}, nil)
	remoteplan.Register(server, config.planExecutor)
	remotecheck.Register(server, config.checkExecutor)

	return server
}
