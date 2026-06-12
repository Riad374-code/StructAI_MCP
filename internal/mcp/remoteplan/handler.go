// Package remoteplan exposes architect_plan as a thin MCP-to-HTTP adapter.
package remoteplan

import (
	"context"
	"fmt"

	plancontract "github.com/Riad374-code/architectmcp/internal/contracts/plan"
	"github.com/Riad374-code/architectmcp/internal/mcp/session"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Executor sends one architect_plan invocation to the backend business layer.
type Executor interface {
	ExecutePlan(
		ctx context.Context,
		backendToken string,
		input plancontract.Input,
	) (plancontract.Output, error)
}

// Register wires the remote architect_plan adapter into the MCP server.
func Register(server *sdk.Server, executor Executor) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        plancontract.ToolName,
		Description: "Turn a raw product idea into a versioned, machine-checkable architecture spec at .architect/spec.json.",
	}, handler(executor))
}

func handler(executor Executor) sdk.ToolHandlerFor[plancontract.Input, plancontract.Output] {
	return func(
		ctx context.Context,
		_ *sdk.CallToolRequest,
		input plancontract.Input,
	) (*sdk.CallToolResult, plancontract.Output, error) {
		if executor == nil {
			return nil, plancontract.Output{}, fmt.Errorf("architect_plan backend is not configured")
		}
		principal, ok := session.PrincipalFromContext(ctx)
		if !ok || principal.BackendToken == "" {
			return nil, plancontract.Output{}, fmt.Errorf("authenticated backend session is missing")
		}
		output, err := executor.ExecutePlan(ctx, principal.BackendToken, input)
		if err != nil {
			return nil, plancontract.Output{}, err
		}
		return nil, output, nil
	}
}
