// Package check defines the architect_check wire contract shared by the MCP
// adapter and backend-owned checking business logic.
package check

import "github.com/Riad374-code/architectmcp/internal/spec"

const (
	ToolName            = "architect_check"
	ToolContractVersion = "1.0"
	InputSchemaVersion  = "1.0"
	OutputSchemaVersion = "1.0"
)

type Status string

const (
	StatusOK         Status = "ok"
	StatusDriftFound Status = "drift_found"
	StatusBlocked    Status = "blocked"
	StatusError      Status = "error"
)

type Input struct {
	Spec             spec.Spec         `json:"spec" jsonschema:"the complete .architect/spec.json object"`
	Graph            map[string]any    `json:"graph" jsonschema:"the complete project-relative graphify-out/graph.json object"`
	DeclaredPackages []DeclaredPackage `json:"declared_packages,omitempty" jsonschema:"dependencies collected from project manifests"`
	Baseline         []BaselineEntry   `json:"baseline,omitempty" jsonschema:"accepted violations from .architect/baseline.json"`
	PhaseOverride    *int              `json:"phase_override,omitempty"`
	IncludeHints     bool              `json:"include_hints,omitempty"`
	Strict           bool              `json:"strict,omitempty"`
	UpdateBaseline   bool              `json:"update_baseline,omitempty"`
	BaselineReason   string            `json:"baseline_reason,omitempty"`
	GraphifyVersion  string            `json:"graphify_version,omitempty"`
}

type DeclaredPackage struct {
	Name      string `json:"name"`
	Path      string `json:"path,omitempty"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
}

type BaselineEntry struct {
	ViolationID string `json:"violation_id"`
	Reason      string `json:"reason"`
}

type Output struct {
	Status           Status            `json:"status"`
	ReportID         string            `json:"report_id,omitempty"`
	Report           map[string]any    `json:"report,omitempty"`
	AgentInstruction string            `json:"agent_instruction"`
	ReportArtifact   *JSONArtifact     `json:"report_artifact,omitempty"`
	BaselineArtifact *BaselineArtifact `json:"baseline_artifact,omitempty"`
	GraphifyVersion  string            `json:"graphify_version,omitempty"`
	Metadata         ToolMetadata      `json:"metadata"`
	Trend            *ReportTrend      `json:"trend,omitempty"`
	Warnings         []string          `json:"warnings"`
	Error            *ToolError        `json:"error,omitempty"`
}

// ToolMetadata pins every independently versioned contract that contributed
// to an architect_check response. Runtime Graphify version stays separate.
type ToolMetadata struct {
	ToolName                   string `json:"tool_name"`
	ToolContractVersion        string `json:"tool_contract_version"`
	InputSchemaVersion         string `json:"input_schema_version"`
	OutputSchemaVersion        string `json:"output_schema_version"`
	SpecSchemaVersion          string `json:"spec_schema_version"`
	ReportSchemaVersion        string `json:"report_schema_version"`
	GraphifyAdapterVersion     string `json:"graphify_adapter_version"`
	EngineCheckRegistryVersion string `json:"engine_check_registry_version"`
}

// ReportTrend is optional backend-derived comparison data. It is never part of
// the deterministic engine report because it depends on an explicit prior run.
type ReportTrend struct {
	PreviousReportID   string `json:"previous_report_id"`
	PreviousScore      int    `json:"previous_score"`
	ScoreDelta         int    `json:"score_delta"`
	NewViolations      int    `json:"new_violations"`
	ResolvedViolations int    `json:"resolved_violations"`
}

type JSONArtifact struct {
	SavePath string `json:"save_path"`
	Content  string `json:"content"`
}

type BaselineArtifact struct {
	SavePath string          `json:"save_path"`
	Entries  []BaselineEntry `json:"entries"`
	Content  string          `json:"content"`
}

type ErrorCode string

const (
	ErrorMissingSpec             ErrorCode = "missing_spec"
	ErrorInvalidSpec             ErrorCode = "invalid_spec"
	ErrorInvalidBaseline         ErrorCode = "invalid_baseline"
	ErrorInvalidProjectPath      ErrorCode = "invalid_project_path"
	ErrorGraphifyNotFound        ErrorCode = "graphify_not_found"
	ErrorGraphifyTimeout         ErrorCode = "graphify_timeout"
	ErrorGraphifyFailed          ErrorCode = "graphify_failed"
	ErrorPartialGraph            ErrorCode = "partial_graph"
	ErrorMalformedGraph          ErrorCode = "malformed_graph"
	ErrorEngineValidation        ErrorCode = "engine_validation_error"
	ErrorBackendMirrorFailed     ErrorCode = "backend_mirror_failed"
	ErrorLLMUnavailable          ErrorCode = "llm_unavailable"
	ErrorLLMInvalidOutput        ErrorCode = "llm_invalid_output"
	ErrorInvalidRequest          ErrorCode = "invalid_request"
	ErrorInvalidDeclaredPackages ErrorCode = "invalid_declared_packages"
	ErrorReportEncoding          ErrorCode = "report_encoding_failed"
	ErrorReportValidation        ErrorCode = "report_validation_failed"
	ErrorBaselineEncoding        ErrorCode = "baseline_encoding_failed"
)

type ErrorStage string

const (
	StageValidation       ErrorStage = "validation"
	StageSpecLoading      ErrorStage = "spec_loading"
	StageGraphify         ErrorStage = "graphify"
	StageGraphAdaptation  ErrorStage = "graph_adaptation"
	StageEngine           ErrorStage = "engine"
	StageReportEncoding   ErrorStage = "report_encoding"
	StageReportValidation ErrorStage = "report_validation"
	StageBaselineEncoding ErrorStage = "baseline_encoding"
	StageBackendMirror    ErrorStage = "backend_mirror"
	StageLLM              ErrorStage = "llm"
)

type ToolError struct {
	Code        ErrorCode  `json:"code"`
	Message     string     `json:"message"`
	Remediation string     `json:"remediation"`
	Retryable   bool       `json:"retryable"`
	Stage       ErrorStage `json:"stage"`
}
