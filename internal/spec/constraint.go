package spec

// Constraint is one explicit, machine-checkable rule. It is a tagged
// union: Kind selects which fields apply, and kindSpecs (below) is the
// single source of truth for which fields each kind requires. A
// constraint whose kind has no engine check mapping is rejected — the
// spec never stores rules the engine cannot evaluate.
type Constraint struct {
	ID   string         `json:"id"`
	Kind ConstraintKind `json:"kind"`

	// Edge-shaped kinds (allowed_dependency, must_call) connect two
	// modules; layer_violation connects two layers.
	FromModule string `json:"from_module,omitempty"`
	ToModule   string `json:"to_module,omitempty"`
	FromLayer  string `json:"from_layer,omitempty"`
	ToLayer    string `json:"to_layer,omitempty"`

	// Node-shaped kinds (required_export, required_module,
	// coupling_threshold) target one module.
	Module string `json:"module,omitempty"`
	// Export names the symbol a required_export constraint demands.
	Export string `json:"export,omitempty"`
	// Dependency names the external package a forbidden_dependency
	// constraint bans.
	Dependency string `json:"dependency,omitempty"`
	// Threshold caps fan-in for coupling_threshold. Zero means unset.
	Threshold int `json:"threshold,omitempty"`

	Severity Severity `json:"severity"`
	Message  string   `json:"message,omitempty"`
}

// ConstraintKind is the closed set of v1 rule kinds. Each kind maps
// one-to-one to an engine check family (Part 05); unknown kinds are
// rejected at validation, not silently skipped.
type ConstraintKind string

// v1 constraint kinds.
const (
	KindForbiddenDependency ConstraintKind = "forbidden_dependency"
	KindAllowedDependency   ConstraintKind = "allowed_dependency"
	KindLayerViolation      ConstraintKind = "layer_violation"
	KindMustCall            ConstraintKind = "must_call"
	KindRequiredExport      ConstraintKind = "required_export"
	KindRequiredModule      ConstraintKind = "required_module"
	KindCouplingThreshold   ConstraintKind = "coupling_threshold"
)

// constraintField names one union field of Constraint, in the same
// spelling the JSON document uses so validation errors point at real
// fields.
type constraintField string

const (
	fieldFromModule constraintField = "from_module"
	fieldToModule   constraintField = "to_module"
	fieldFromLayer  constraintField = "from_layer"
	fieldToLayer    constraintField = "to_layer"
	fieldModule     constraintField = "module"
	fieldExport     constraintField = "export"
	fieldDependency constraintField = "dependency"
	fieldThreshold  constraintField = "threshold"
)

// unionFields lists every union field in a fixed order so validation
// emits errors deterministically.
var unionFields = []struct {
	name  constraintField
	isSet func(Constraint) bool
}{
	{fieldFromModule, func(c Constraint) bool { return c.FromModule != "" }},
	{fieldToModule, func(c Constraint) bool { return c.ToModule != "" }},
	{fieldFromLayer, func(c Constraint) bool { return c.FromLayer != "" }},
	{fieldToLayer, func(c Constraint) bool { return c.ToLayer != "" }},
	{fieldModule, func(c Constraint) bool { return c.Module != "" }},
	{fieldExport, func(c Constraint) bool { return c.Export != "" }},
	{fieldDependency, func(c Constraint) bool { return c.Dependency != "" }},
	{fieldThreshold, func(c Constraint) bool { return c.Threshold != 0 }},
}

// kindSpec declares, for one constraint kind, the engine check family
// that enforces it and the union fields it requires/permits. Any set
// field outside required+optional is rejected: a misplaced field means
// the author intended a rule the engine would silently not enforce.
type kindSpec struct {
	family   string
	required []constraintField
	optional []constraintField
}

// kindSpecs is the closed kind set and the kind→check-family mapping
// in one table. Family names match the Part 05 check battery:
// dependency_diff, layer_scan, tech_stack, module_completeness,
// cycle_detection, interface_contracts, coupling, must_call.
var kindSpecs = map[ConstraintKind]kindSpec{
	KindForbiddenDependency: {
		family:   "dependency_diff",
		required: []constraintField{fieldDependency},
		// FromModule narrows the ban to one module; absent means
		// project-wide.
		optional: []constraintField{fieldFromModule},
	},
	KindAllowedDependency: {
		family:   "dependency_diff",
		required: []constraintField{fieldFromModule, fieldToModule},
	},
	KindLayerViolation: {
		family:   "layer_scan",
		required: []constraintField{fieldFromLayer, fieldToLayer},
	},
	KindMustCall: {
		family:   "must_call",
		required: []constraintField{fieldFromModule, fieldToModule},
	},
	KindRequiredExport: {
		family:   "interface_contracts",
		required: []constraintField{fieldModule, fieldExport},
	},
	KindRequiredModule: {
		family:   "module_completeness",
		required: []constraintField{fieldModule},
	},
	KindCouplingThreshold: {
		family:   "coupling",
		required: []constraintField{fieldModule, fieldThreshold},
	},
}

// constraintKinds lists the closed set in a fixed order for
// deterministic error messages and docs.
var constraintKinds = []ConstraintKind{
	KindForbiddenDependency,
	KindAllowedDependency,
	KindLayerViolation,
	KindMustCall,
	KindRequiredExport,
	KindRequiredModule,
	KindCouplingThreshold,
}

// CheckFamily returns the engine check family that enforces kind, and
// whether the kind is part of the closed v1 set.
func CheckFamily(kind ConstraintKind) (string, bool) {
	ks, ok := kindSpecs[kind]
	return ks.family, ok
}
