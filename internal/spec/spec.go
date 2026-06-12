// Package spec owns the versioned architecture spec contract: the Go
// structs, JSON Schema validation, semantic validation, and atomic
// local persistence at .architect/spec.json.
//
// The spec is not documentation — it is a compiled rule set the
// deterministic engine evaluates against a code graph. Every field
// here is machine-checkable: it compiles to a predicate over graph
// nodes, graph edges, file paths, packages, exports, or phases.
// Prose-only architectural rules are not accepted as constraints.
//
// Determinism contract for this package:
//   - All rule collections are slices, never maps, so evaluation and
//     marshaling order is the explicit document order.
//   - Marshaling uses struct field declaration order (encoding/json),
//     so identical specs marshal byte-identically.
//   - tech_stack.required is an ordered array of {category, choice}
//     pairs instead of the draft's JSON object: object keys carry no
//     reliable order and would force map iteration in checks.
package spec

// CanonicalRelPath is where the canonical spec lives, relative to the
// user project root. It is local-first and version-controlled with the
// user project; any backend copy is only a projection of this file.
const CanonicalRelPath = ".architect/spec.json"

// ReportRelPath is where architect_check persists its last report,
// relative to the user project root, for trend/delta support.
const ReportRelPath = ".architect/last-report.json"

// SchemaVersion is the spec format version this build understands.
// Loading a spec with any other schema_version fails validation until
// a migration path exists (see Migration).
const SchemaVersion = "1.0"

// Spec is the machine-checkable architecture contract produced by
// architect_plan and consumed by architect_check.
//
// Top-level rule collections intentionally omit `omitempty`: a saved
// spec always carries every section (empty sections marshal as []),
// which keeps file diffs predictable and makes "no rules" explicit
// rather than ambiguous with "section missing".
type Spec struct {
	// SchemaVersion identifies the spec format itself.
	SchemaVersion string `json:"schema_version"`
	// SpecVersion increases monotonically with each spec revision.
	SpecVersion int `json:"spec_version"`
	// SpecID is a stable identifier for the project's spec lineage.
	SpecID string `json:"spec_id"`
	// Migration carries optional metadata when a spec has been
	// upgraded from an earlier schema version.
	Migration *Migration `json:"migration,omitempty"`

	Project    Project     `json:"project"`
	Workspaces []Workspace `json:"workspaces"`
	TechStack  TechStack   `json:"tech_stack"`
	Ignore     Ignore      `json:"ignore"`

	Layers     []Layer     `json:"layers"`
	LayerRules []LayerRule `json:"layer_rules"`
	// LayerDefault is applied only when no explicit layer rule or
	// layer_violation constraint matches a cross-layer edge.
	LayerDefault *LayerDefaultRule `json:"layer_default,omitempty"`
	Modules      []Module          `json:"modules"`
	Phases       []Phase           `json:"phases"`
	// Constraints are explicit rules that map one-to-one to engine
	// checks. LayerRule and constraint IDs share one namespace because
	// both surface as Violation.RuleRef values.
	Constraints []Constraint `json:"constraints"`
}

// Migration is optional metadata recorded when a spec is upgraded
// across schema versions. v1 records provenance only; migration logic
// itself lands with the first schema bump.
type Migration struct {
	FromSchemaVersion string `json:"from_schema_version"`
	Note              string `json:"note,omitempty"`
}

// Project is identity and planning context. Name is identity;
// Description and Assumptions are display/handoff context and are
// never evaluated by the engine.
type Project struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Assumptions []string `json:"assumptions,omitempty"`
}

// Workspace is one package root in the repository. Single-package
// repos declare exactly one workspace with root ".". Roots use forward
// slashes and are project-root-relative.
//
// Resolution rule: when workspace roots nest, the longest matching
// root wins. Two workspaces with the same cleaned root are rejected as
// ambiguous.
type Workspace struct {
	ID   string `json:"id"`
	Root string `json:"root"`
	// Language and PackageManager inform graph adaptation and manifest
	// parsing (e.g. which lockfile declares dependencies).
	Language       string `json:"language,omitempty"`
	PackageManager string `json:"package_manager,omitempty"`
}

// TechStack pins technology choices the engine can verify against
// declared dependencies in the graph.
type TechStack struct {
	// Required is ordered; at most one choice per category.
	Required []TechRequirement `json:"required"`
	// Forbidden lists dependency names that must not appear in any
	// manifest or import.
	Forbidden []string `json:"forbidden"`
	// Rationales explain choices for handoff display; the engine never
	// evaluates them.
	Rationales []Rationale `json:"rationales"`
}

// TechRequirement pins one category to one choice, e.g. {"orm",
// "drizzle"}.
type TechRequirement struct {
	Category string `json:"category"`
	Choice   string `json:"choice"`
	// Packages optionally lists the manifest package names that
	// satisfy this choice when they differ from the choice itself
	// (e.g. choice "drizzle" ships as the npm package "drizzle-orm").
	// Empty means the choice name is the package name. The engine's
	// tech-stack check matches any listed name against declared
	// dependencies instead of guessing from the choice.
	Packages []string `json:"packages,omitempty"`
}

// Rationale records why a tech choice was made.
type Rationale struct {
	Choice string `json:"choice"`
	Reason string `json:"reason"`
}

// Ignore excludes paths from all checks: tests, generated files,
// vendored code, and other intentional exclusions.
type Ignore struct {
	Paths []IgnoreEntry `json:"paths"`
	// ReasonRequired makes Reason mandatory on every entry, so
	// exclusions stay auditable instead of silently growing.
	ReasonRequired bool `json:"reason_required,omitempty"`
}

// IgnoreEntry is one exclusion glob. Entries are objects rather than
// bare strings so reason_required is machine-checkable.
type IgnoreEntry struct {
	Pattern string `json:"pattern"`
	Reason  string `json:"reason,omitempty"`
}

// Layer is a horizontal slice of the codebase defined by glob
// patterns. Order gives layers a deterministic total order (lower =
// closer to the user); duplicate orders are rejected.
type Layer struct {
	ID    string   `json:"id"`
	Paths []string `json:"paths"`
	Order int      `json:"order"`
}

// LayerRule allows or forbids edges from one layer to another. Rules
// with Allow=false compile to layer_violation checks; rules with
// Allow=true are explicit exceptions consulted before defaults.
type LayerRule struct {
	ID       string   `json:"id"`
	From     string   `json:"from"`
	To       string   `json:"to"`
	Allow    bool     `json:"allow"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message,omitempty"`
}

// LayerDefaultRule defines the conservative fallback for otherwise-unmatched
// cross-layer edges. Nil means the spec intentionally has no default policy.
type LayerDefaultRule struct {
	ID       string   `json:"id"`
	Allow    bool     `json:"allow"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message,omitempty"`
}

// Module is a vertical feature slice with a declared responsibility,
// phase, and dependency contract.
type Module struct {
	ID string `json:"id"`
	// Package scopes Paths to a workspace (references Workspace.ID).
	// Empty means Paths are project-root-relative.
	Package string `json:"package,omitempty"`
	// Phase references Phase.ID; future-phase modules do not produce
	// completeness violations.
	Phase int      `json:"phase"`
	Paths []string `json:"paths"`
	// Responsibility is display/handoff context, never evaluated.
	Responsibility string `json:"responsibility,omitempty"`
	// AllowEmpty permits a planned module to count as present before files are
	// created. It should be used sparingly and only for explicit placeholders.
	AllowEmpty bool `json:"allow_empty,omitempty"`
	// AllowedDeps lists module IDs this module may depend on. The
	// absent/empty distinction is meaningful, so this is a pointer:
	// nil (field absent in JSON) leaves dependencies unconstrained,
	// while a present-but-empty list closes the module — it may depend
	// on nothing. To constrain a module, list its full allowed set.
	AllowedDeps *[]string `json:"allowed_deps,omitempty"`
	// RequiredExports must each resolve to an exported symbol inside
	// this module's paths.
	RequiredExports []string `json:"required_exports,omitempty"`
	// CouplingThreshold caps how many distinct modules may depend on
	// this module. Zero means no threshold — a threshold of zero
	// ("nothing may depend on this") is intentionally inexpressible
	// here; that intent belongs in dependents' AllowedDeps instead.
	CouplingThreshold int `json:"coupling_threshold,omitempty"`
}

// Phase is one step of the delivery plan. The checker suppresses
// completeness violations for modules in phases later than the active
// one.
type Phase struct {
	ID              int      `json:"id"`
	Name            string   `json:"name"`
	RequiredModules []string `json:"required_modules"`
}

// Severity grades how strongly a rule should gate the agent. It is
// declared on spec rules here and echoed verbatim into report
// violations (the engine aliases this type), so the two planes cannot
// drift.
type Severity string

// Severity levels, ordered from most to least blocking.
const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
)

// validSeverity reports whether s is one of the closed severity set.
// Semantic validation must check this even though the JSON Schema
// carries the enum, because specs are also constructed directly in Go
// by architect_plan.
func validSeverity(s Severity) bool {
	switch s {
	case SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow:
		return true
	}
	return false
}
