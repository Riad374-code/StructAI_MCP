// Package plan defines the architect_plan wire contract shared by the MCP
// adapter and the backend business-logic service.
package plan

import "github.com/Riad374-code/architectmcp/internal/spec"

// ToolName is the MCP-visible tool name and backend routing key.
const (
	ToolName            = "architect_plan"
	ToolContractVersion = "1.0"
	InputSchemaVersion  = "1.0"
	OutputSchemaVersion = "1.0"
)

type Input struct {
	RawIdea string `json:"raw_idea,omitempty" jsonschema:"the raw product idea to turn into a machine-checkable architecture spec; required when creating a new spec"`

	TargetPlatform string   `json:"target_platform,omitempty" jsonschema:"optional target platform, e.g. web, mobile, desktop, cli, api-only"`
	ExistingStack  []string `json:"existing_stack,omitempty" jsonschema:"optional technologies the architecture must keep using"`
	Constraints    []string `json:"constraints,omitempty" jsonschema:"optional hard constraints (compliance, budget, hosting, team skills)"`
	RequestedMVP   string   `json:"requested_mvp,omitempty" jsonschema:"optional requested MVP boundary: what phase 1 must and must not include"`

	PlanningSessionID      string   `json:"planning_session_id,omitempty" jsonschema:"echo of the planning_session_id from an earlier needs_input response"`
	Answers                []Answer `json:"answers,omitempty" jsonschema:"answers to clarifying questions from an earlier needs_input response"`
	ProceedWithAssumptions bool     `json:"proceed_with_assumptions,omitempty" jsonschema:"skip remaining clarifying questions and proceed with recorded assumptions"`

	DomainModel       map[string]any `json:"domain_model,omitempty" jsonschema:"the generated domain model JSON requested by a generation_request of kind domain_model"`
	Architecture      map[string]any `json:"architecture,omitempty" jsonschema:"the generated architecture draft JSON requested by a generation_request of kind architecture"`
	GenerationAttempt int            `json:"generation_attempt,omitempty" jsonschema:"echo of generation_request.attempt when supplying a generated payload"`

	ExistingSpec    *spec.Spec `json:"existing_spec,omitempty" jsonschema:"the current .architect/spec.json content when requesting a revision"`
	RevisionRequest string     `json:"revision_request,omitempty" jsonschema:"what to change and why when existing_spec is supplied"`
	SpecDelta       *SpecDelta `json:"spec_delta,omitempty" jsonschema:"the minimal spec change set requested by a generation_request of kind spec_delta"`

	BillingOperationID string `json:"billing_operation_id,omitempty" jsonschema:"opaque idempotency key returned by architect_plan and echoed across this planning operation"`
}

type Answer struct {
	QuestionID string `json:"question_id" jsonschema:"the id of the question being answered"`
	Answer     string `json:"answer" jsonschema:"the answer text; use 'unknown' to accept the recorded default assumption"`
}

type Status string

const (
	StatusNeedsInput  Status = "needs_input"
	StatusSpecCreated Status = "spec_created"
	StatusSpecRevised Status = "spec_revised"
)

type Output struct {
	Status             Status             `json:"status"`
	PlanningSessionID  string             `json:"planning_session_id,omitempty"`
	BillingOperationID string             `json:"billing_operation_id,omitempty"`
	Message            string             `json:"message,omitempty"`
	Questions          []Question         `json:"questions,omitempty"`
	Assumptions        []string           `json:"assumptions,omitempty"`
	GenerationRequest  *GenerationRequest `json:"generation_request,omitempty"`
	Spec               *spec.Spec         `json:"spec,omitempty"`
	SavePath           string             `json:"save_path,omitempty"`
	SpecID             string             `json:"spec_id,omitempty"`
	SpecVersion        int                `json:"spec_version,omitempty"`
	SpecSummary        *SpecSummary       `json:"spec_summary,omitempty"`
	PhasePlan          []PhaseStep        `json:"phase_plan,omitempty"`
	FirstPhaseTasks    []Task             `json:"first_phase_tasks,omitempty"`
	RevisionSummary    *RevisionDelta     `json:"revision_summary,omitempty"`
	Warnings           []string           `json:"warnings,omitempty"`
}

type Question struct {
	ID           string `json:"id"`
	DecisionArea string `json:"decision_area"`
	Question     string `json:"question"`
	Why          string `json:"why"`
	Default      string `json:"default"`
}

type GenerationRequest struct {
	Kind             string                 `json:"kind"`
	Attempt          int                    `json:"attempt"`
	MaxAttempts      int                    `json:"max_attempts"`
	Instructions     string                 `json:"instructions"`
	OutputSchema     map[string]any         `json:"output_schema,omitempty"`
	ValidationErrors []spec.ValidationError `json:"validation_errors,omitempty"`
}

type SpecSummary struct {
	ProjectName     string   `json:"project_name"`
	Description     string   `json:"description,omitempty"`
	Workspaces      int      `json:"workspaces"`
	Layers          int      `json:"layers"`
	Modules         int      `json:"modules"`
	Phases          int      `json:"phases"`
	LayerRules      int      `json:"layer_rules"`
	Constraints     int      `json:"constraints"`
	TechStack       []string `json:"tech_stack,omitempty"`
	AssumptionCount int      `json:"assumption_count"`
}

type PhaseStep struct {
	Phase    int      `json:"phase"`
	Name     string   `json:"name"`
	Modules  []string `json:"modules"`
	BuildNow bool     `json:"build_now"`
}

type Task struct {
	Order  int    `json:"order"`
	Title  string `json:"title"`
	Module string `json:"module,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type RevisionDelta struct {
	FromVersion int      `json:"from_version"`
	ToVersion   int      `json:"to_version"`
	Reason      string   `json:"reason"`
	Added       []string `json:"added"`
	Changed     []string `json:"changed"`
	Removed     []string `json:"removed"`
}

type SpecDelta struct {
	Project           *ProjectDelta          `json:"project,omitempty"`
	TechStack         *spec.TechStack        `json:"tech_stack,omitempty"`
	Ignore            *spec.Ignore           `json:"ignore,omitempty"`
	LayerDefault      *spec.LayerDefaultRule `json:"layer_default,omitempty"`
	ClearLayerDefault bool                   `json:"clear_layer_default,omitempty"`

	UpsertWorkspaces    []spec.Workspace  `json:"upsert_workspaces,omitempty"`
	RemoveWorkspaceIDs  []string          `json:"remove_workspace_ids,omitempty"`
	UpsertLayers        []spec.Layer      `json:"upsert_layers,omitempty"`
	RemoveLayerIDs      []string          `json:"remove_layer_ids,omitempty"`
	UpsertLayerRules    []spec.LayerRule  `json:"upsert_layer_rules,omitempty"`
	RemoveLayerRuleIDs  []string          `json:"remove_layer_rule_ids,omitempty"`
	UpsertModules       []spec.Module     `json:"upsert_modules,omitempty"`
	RemoveModuleIDs     []string          `json:"remove_module_ids,omitempty"`
	UpsertPhases        []spec.Phase      `json:"upsert_phases,omitempty"`
	RemovePhaseIDs      []int             `json:"remove_phase_ids,omitempty"`
	UpsertConstraints   []spec.Constraint `json:"upsert_constraints,omitempty"`
	RemoveConstraintIDs []string          `json:"remove_constraint_ids,omitempty"`
}

type ProjectDelta struct {
	Name              *string  `json:"name,omitempty"`
	Description       *string  `json:"description,omitempty"`
	AddAssumptions    []string `json:"add_assumptions,omitempty"`
	RemoveAssumptions []string `json:"remove_assumptions,omitempty"`
}
