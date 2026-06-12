package remoteplan

import (
	"context"
	"testing"

	plancontract "github.com/Riad374-code/architectmcp/internal/contracts/plan"
	"github.com/Riad374-code/architectmcp/internal/mcp/session"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type executorFunc func(context.Context, string, plancontract.Input) (plancontract.Output, error)

func (f executorFunc) ExecutePlan(
	ctx context.Context,
	token string,
	input plancontract.Input,
) (plancontract.Output, error) {
	return f(ctx, token, input)
}

func TestHandlerDelegatesToBackendExecutor(t *testing.T) {
	var gotToken string
	var gotInput plancontract.Input
	executor := executorFunc(func(
		_ context.Context,
		token string,
		input plancontract.Input,
	) (plancontract.Output, error) {
		gotToken = token
		gotInput = input
		return plancontract.Output{Status: plancontract.StatusNeedsInput}, nil
	})
	ctx := session.WithPrincipal(context.Background(), session.Principal{
		BackendToken: "session-jwt",
	})

	_, output, err := handler(executor)(ctx, nil, plancontract.Input{
		RawIdea: "A collaborative planning workspace",
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if gotToken != "session-jwt" {
		t.Errorf("token = %q, want session-jwt", gotToken)
	}
	if gotInput.RawIdea != "A collaborative planning workspace" {
		t.Errorf("input = %#v", gotInput)
	}
	if output.Status != plancontract.StatusNeedsInput {
		t.Errorf("status = %q, want needs_input", output.Status)
	}
}

func TestRegisterDoesNotPanic(t *testing.T) {
	server := sdk.NewServer(&sdk.Implementation{Name: "test", Version: "0"}, nil)
	Register(server, executorFunc(func(
		context.Context,
		string,
		plancontract.Input,
	) (plancontract.Output, error) {
		return plancontract.Output{}, nil
	}))
}
