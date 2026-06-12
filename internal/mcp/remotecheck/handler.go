// Package remotecheck exposes architect_check as a thin MCP-to-HTTP adapter.
package remotecheck

import (
	"context"
	"fmt"

	checkcontract "github.com/Riad374-code/architectmcp/internal/contracts/check"
	"github.com/Riad374-code/architectmcp/internal/mcp/session"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Executor sends one architect_check invocation to backend business logic.
type Executor interface {
	ExecuteCheck(
		ctx context.Context,
		backendToken string,
		input checkcontract.Input,
	) (checkcontract.Output, error)
}

// Register wires the remote architect_check adapter into the hosted server.
func Register(server *sdk.Server, executor Executor) {
	sdk.AddTool(server, &sdk.Tool{
		Name: checkcontract.ToolName,
		Description: "Check a project against its architecture spec. Supply the complete " +
			".architect/spec.json and project-relative graphify-out/graph.json objects; " +
			"save returned artifacts at their provided paths.",
	}, handler(executor))
}

func handler(executor Executor) sdk.ToolHandlerFor[checkcontract.Input, checkcontract.Output] {
	return func(
		ctx context.Context,
		_ *sdk.CallToolRequest,
		input checkcontract.Input,
	) (*sdk.CallToolResult, checkcontract.Output, error) {
		if executor == nil {
			return nil, checkcontract.Output{}, fmt.Errorf("architect_check backend is not configured")
		}
		principal, ok := session.PrincipalFromContext(ctx)
		if !ok || principal.BackendToken == "" {
			return nil, checkcontract.Output{}, fmt.Errorf("authenticated backend session is missing")
		}
		output, err := executor.ExecuteCheck(ctx, principal.BackendToken, input)
		if err != nil {
			return nil, checkcontract.Output{}, err
		}
		return nil, output, nil
	}
}
