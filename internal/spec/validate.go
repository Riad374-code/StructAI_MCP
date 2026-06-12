package spec

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// ValidationError is one semantic problem in a spec. Path points at
// the offending JSON location, Rule is a stable machine-readable rule
// identifier, and Message explains the problem for humans (and for the
// architect_plan elicitation loop).
type ValidationError struct {
	Path    string `json:"path"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// ValidationErrors collects every problem found in one pass, in
// document order, so callers can fix a spec in one round trip instead
// of replaying validation error by error.
type ValidationErrors []ValidationError

// Error renders all collected problems, one per line.
func (e ValidationErrors) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "spec validation failed: %d issue(s)", len(e))
	for _, ve := range e {
		fmt.Fprintf(&b, "\n  %s: %s [%s]", ve.Path, ve.Message, ve.Rule)
	}
	return b.String()
}

// idPattern constrains every spec identifier: lowercase start, then
// lowercase alphanumerics, '-' or '_', max 64 chars. Stable IDs feed
// violation rule_refs, so they must stay machine-friendly.
var idPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// driveLetterPattern detects Windows-style absolute paths like "C:/x".
var driveLetterPattern = regexp.MustCompile(`^[a-zA-Z]:`)

// Validate applies every semantic rule that the JSON Schema cannot
// express: cross-field references, uniqueness, contradictions, and
// path-pattern safety. It runs on Go-constructed specs too, so it
// re-checks closed sets (severity, kind) the schema also carries.
func (s Spec) Validate() error {
	v := &validator{idx: buildIndex(s), ruleIDs: make(map[string]bool)}
	v.identity(s)
	v.workspaces(s)
	v.techStack(s)
	v.ignore(s)
	v.layers(s)
	v.layerRules(s)
	v.layerDefault(s)
	v.modules(s)
	v.phases(s)
	v.constraints(s)
	if len(v.errs) == 0 {
		return nil
	}
	return v.errs
}

// refIndex holds the declared-ID sets cross-references resolve
// against. Maps are lookup-only — never iterated — so they cannot
// introduce ordering nondeterminism.
type refIndex struct {
	workspaceIDs map[string]bool
	layerIDs     map[string]bool
	moduleIDs    map[string]bool
	phaseIDs     map[int]bool
}

func buildIndex(s Spec) refIndex {
	idx := refIndex{
		workspaceIDs: make(map[string]bool, len(s.Workspaces)),
		layerIDs:     make(map[string]bool, len(s.Layers)),
		moduleIDs:    make(map[string]bool, len(s.Modules)),
		phaseIDs:     make(map[int]bool, len(s.Phases)),
	}
	for _, w := range s.Workspaces {
		idx.workspaceIDs[w.ID] = true
	}
	for _, l := range s.Layers {
		idx.layerIDs[l.ID] = true
	}
	for _, m := range s.Modules {
		idx.moduleIDs[m.ID] = true
	}
	for _, p := range s.Phases {
		idx.phaseIDs[p.ID] = true
	}
	return idx
}

type validator struct {
	idx  refIndex
	errs ValidationErrors
	// ruleIDs is the shared ID namespace for layer rules and
	// constraints: both surface as Violation.RuleRef values, so a
	// collision would make a rule_ref ambiguous.
	ruleIDs map[string]bool
}

func (v *validator) add(path, rule, message string) {
	v.errs = append(v.errs, ValidationError{Path: path, Rule: rule, Message: message})
}

// checkID validates one identifier and tracks uniqueness within the
// given namespace. Each namespace passes its own seen-set.
func (v *validator) checkID(path, id string, seen map[string]bool) {
	if !idPattern.MatchString(id) {
		v.add(path, "invalid_id", fmt.Sprintf("%q must match %s", id, idPattern.String()))
		return
	}
	if seen[id] {
		v.add(path, "duplicate_id", fmt.Sprintf("%q is declared more than once", id))
		return
	}
	seen[id] = true
}

// checkGlob validates one glob pattern: non-empty, forward slashes,
// project-relative, no parent escapes, valid doublestar syntax.
func (v *validator) checkGlob(path, pattern string) {
	if msg, ok := relPathProblem(pattern); !ok {
		v.add(path, "invalid_glob", msg)
		return
	}
	if !doublestar.ValidatePattern(pattern) {
		v.add(path, "invalid_glob", fmt.Sprintf("%q is not a valid glob pattern", pattern))
	}
}

// relPathProblem reports why p is unsafe as a project-relative path or
// pattern, if it is.
func relPathProblem(p string) (string, bool) {
	switch {
	case p == "":
		return "path pattern is empty", false
	case strings.Contains(p, `\`):
		return fmt.Sprintf("%q must use forward slashes", p), false
	case strings.HasPrefix(p, "/") || driveLetterPattern.MatchString(p):
		return fmt.Sprintf("%q must be relative to the project root", p), false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return fmt.Sprintf("%q must not escape the project root", p), false
		}
	}
	return "", true
}

func (v *validator) identity(s Spec) {
	if s.SchemaVersion != SchemaVersion {
		v.add("schema_version", "unsupported_schema_version",
			fmt.Sprintf("%q is not supported; this build understands %q", s.SchemaVersion, SchemaVersion))
	}
	if s.SpecVersion < 1 {
		v.add("spec_version", "invalid_spec_version",
			fmt.Sprintf("%d must be a positive integer", s.SpecVersion))
	}
	if !idPattern.MatchString(s.SpecID) {
		v.add("spec_id", "invalid_id",
			fmt.Sprintf("%q must match %s", s.SpecID, idPattern.String()))
	}
	if s.Project.Name == "" {
		v.add("project.name", "missing_project_name", "project name is required")
	}
	if s.Migration != nil && s.Migration.FromSchemaVersion == "" {
		v.add("migration.from_schema_version", "invalid_migration",
			"migration metadata must record the schema version it came from")
	}
}

func (v *validator) workspaces(s Spec) {
	if len(s.Workspaces) == 0 {
		v.add("workspaces", "missing_workspaces",
			"at least one workspace is required; single-package repos declare one workspace with root \".\"")
		return
	}
	seenIDs := make(map[string]bool, len(s.Workspaces))
	seenRoots := make(map[string]string, len(s.Workspaces))
	for i, w := range s.Workspaces {
		p := fmt.Sprintf("workspaces[%d]", i)
		v.checkID(p+".id", w.ID, seenIDs)
		if msg, ok := relPathProblem(w.Root); !ok {
			v.add(p+".root", "invalid_path", msg)
			continue
		}
		cleaned := path.Clean(w.Root)
		if prev, dup := seenRoots[cleaned]; dup {
			v.add(p+".root", "ambiguous_workspace_root",
				fmt.Sprintf("root %q is already claimed by workspace %q; deterministic resolution is impossible", cleaned, prev))
			continue
		}
		seenRoots[cleaned] = w.ID
	}
}

func (v *validator) techStack(s Spec) {
	seenCategories := make(map[string]bool, len(s.TechStack.Required))
	// requiredNames collects every name a requirement claims — the
	// choice plus any manifest package aliases — so the forbidden list
	// can be checked for contradictions against all of them.
	requiredNames := make(map[string]bool, len(s.TechStack.Required))
	for i, r := range s.TechStack.Required {
		p := fmt.Sprintf("tech_stack.required[%d]", i)
		if r.Category == "" || r.Choice == "" {
			v.add(p, "invalid_tech_requirement", "category and choice are both required")
			continue
		}
		if seenCategories[r.Category] {
			v.add(p+".category", "duplicate_tech_category",
				fmt.Sprintf("category %q is pinned more than once", r.Category))
			continue
		}
		seenCategories[r.Category] = true
		requiredNames[r.Choice] = true
		v.techPackages(p, r, requiredNames)
	}
	seenForbidden := make(map[string]bool, len(s.TechStack.Forbidden))
	for i, f := range s.TechStack.Forbidden {
		p := fmt.Sprintf("tech_stack.forbidden[%d]", i)
		if f == "" {
			v.add(p, "invalid_tech_forbidden", "forbidden dependency name is empty")
			continue
		}
		if seenForbidden[f] {
			v.add(p, "duplicate_entry", fmt.Sprintf("%q is forbidden more than once", f))
			continue
		}
		seenForbidden[f] = true
		if requiredNames[f] {
			v.add(p, "contradictory_tech_stack",
				fmt.Sprintf("%q is both required and forbidden", f))
		}
	}
	for i, r := range s.TechStack.Rationales {
		if r.Choice == "" || r.Reason == "" {
			v.add(fmt.Sprintf("tech_stack.rationales[%d]", i), "invalid_rationale",
				"choice and reason are both required")
		}
	}
}

// techPackages validates one requirement's manifest package aliases:
// non-empty, unique within the entry. Each valid alias joins
// requiredNames so the forbidden list is checked against aliases too.
func (v *validator) techPackages(p string, r TechRequirement, requiredNames map[string]bool) {
	seen := make(map[string]bool, len(r.Packages))
	for j, pkg := range r.Packages {
		pp := fmt.Sprintf("%s.packages[%d]", p, j)
		if pkg == "" {
			v.add(pp, "invalid_tech_requirement", "package name is empty")
			continue
		}
		if seen[pkg] {
			v.add(pp, "duplicate_entry", fmt.Sprintf("%q is listed more than once", pkg))
			continue
		}
		seen[pkg] = true
		requiredNames[pkg] = true
	}
}

func (v *validator) ignore(s Spec) {
	seen := make(map[string]bool, len(s.Ignore.Paths))
	for i, e := range s.Ignore.Paths {
		p := fmt.Sprintf("ignore.paths[%d]", i)
		v.checkGlob(p+".pattern", e.Pattern)
		if seen[e.Pattern] {
			v.add(p+".pattern", "duplicate_entry",
				fmt.Sprintf("ignore pattern %q is declared more than once", e.Pattern))
		}
		seen[e.Pattern] = true
		if s.Ignore.ReasonRequired && e.Reason == "" {
			v.add(p+".reason", "missing_ignore_reason",
				fmt.Sprintf("ignore.reason_required is true but %q has no reason", e.Pattern))
		}
	}
}

func (v *validator) layers(s Spec) {
	seenIDs := make(map[string]bool, len(s.Layers))
	seenOrders := make(map[int]string, len(s.Layers))
	for i, l := range s.Layers {
		p := fmt.Sprintf("layers[%d]", i)
		v.checkID(p+".id", l.ID, seenIDs)
		if len(l.Paths) == 0 {
			v.add(p+".paths", "missing_paths", "a layer must declare at least one path pattern")
		}
		for j, g := range l.Paths {
			v.checkGlob(fmt.Sprintf("%s.paths[%d]", p, j), g)
		}
		if prev, dup := seenOrders[l.Order]; dup {
			v.add(p+".order", "duplicate_layer_order",
				fmt.Sprintf("order %d is already used by layer %q; layer order must be a total order", l.Order, prev))
			continue
		}
		seenOrders[l.Order] = l.ID
	}
}

// layerRules validates layer rules. Their IDs share a namespace with
// constraint IDs because both surface as Violation.RuleRef values;
// the shared seen-set lives on the validator.
func (v *validator) layerRules(s Spec) {
	for i, r := range s.LayerRules {
		p := fmt.Sprintf("layer_rules[%d]", i)
		v.checkID(p+".id", r.ID, v.ruleIDs)
		v.checkLayerRef(p+".from", r.From)
		v.checkLayerRef(p+".to", r.To)
		if r.From != "" && r.From == r.To {
			v.add(p, "self_reference", "a layer rule cannot point a layer at itself")
		}
		if !validSeverity(r.Severity) {
			v.add(p+".severity", "invalid_severity",
				fmt.Sprintf("%q is not one of critical, high, medium, low", r.Severity))
		}
	}
}

func (v *validator) layerDefault(s Spec) {
	if s.LayerDefault == nil {
		return
	}
	v.checkID("layer_default.id", s.LayerDefault.ID, v.ruleIDs)
	if !validSeverity(s.LayerDefault.Severity) {
		v.add("layer_default.severity", "invalid_severity",
			fmt.Sprintf("%q is not one of critical, high, medium, low", s.LayerDefault.Severity))
	}
}

func (v *validator) checkLayerRef(path, id string) {
	if !v.idx.layerIDs[id] {
		v.add(path, "unknown_layer_ref", fmt.Sprintf("layer %q is not declared", id))
	}
}

func (v *validator) checkModuleRef(path, id string) {
	if !v.idx.moduleIDs[id] {
		v.add(path, "unknown_module_ref", fmt.Sprintf("module %q is not declared", id))
	}
}

func (v *validator) modules(s Spec) {
	seenIDs := make(map[string]bool, len(s.Modules))
	for i, m := range s.Modules {
		p := fmt.Sprintf("modules[%d]", i)
		v.checkID(p+".id", m.ID, seenIDs)
		if m.Package != "" && !v.idx.workspaceIDs[m.Package] {
			v.add(p+".package", "unknown_workspace_ref",
				fmt.Sprintf("workspace %q is not declared", m.Package))
		}
		if !v.idx.phaseIDs[m.Phase] {
			v.add(p+".phase", "unknown_phase_ref",
				fmt.Sprintf("phase %d is not declared", m.Phase))
		}
		if len(m.Paths) == 0 {
			v.add(p+".paths", "missing_paths", "a module must declare at least one path pattern")
		}
		for j, g := range m.Paths {
			v.checkGlob(fmt.Sprintf("%s.paths[%d]", p, j), g)
		}
		v.moduleDeps(p, m)
		if m.CouplingThreshold < 0 {
			v.add(p+".coupling_threshold", "invalid_threshold",
				fmt.Sprintf("%d must be zero (unset) or a positive integer", m.CouplingThreshold))
		}
	}
}

func (v *validator) moduleDeps(p string, m Module) {
	// AllowedDeps is a pointer because absent and empty differ: nil
	// means unconstrained, empty means the module may depend on
	// nothing. Both validate trivially; only listed deps need checks.
	var deps []string
	if m.AllowedDeps != nil {
		deps = *m.AllowedDeps
	}
	seenDeps := make(map[string]bool, len(deps))
	for j, dep := range deps {
		dp := fmt.Sprintf("%s.allowed_deps[%d]", p, j)
		if dep == m.ID {
			v.add(dp, "self_reference", "a module cannot list itself as an allowed dependency")
			continue
		}
		if seenDeps[dep] {
			v.add(dp, "duplicate_entry", fmt.Sprintf("%q is listed more than once", dep))
			continue
		}
		seenDeps[dep] = true
		v.checkModuleRef(dp, dep)
	}
	seenExports := make(map[string]bool, len(m.RequiredExports))
	for j, exp := range m.RequiredExports {
		ep := fmt.Sprintf("%s.required_exports[%d]", p, j)
		if exp == "" {
			v.add(ep, "invalid_export", "required export name is empty")
			continue
		}
		if seenExports[exp] {
			v.add(ep, "duplicate_entry", fmt.Sprintf("%q is listed more than once", exp))
			continue
		}
		seenExports[exp] = true
	}
}

func (v *validator) phases(s Spec) {
	seenIDs := make(map[int]bool, len(s.Phases))
	for i, ph := range s.Phases {
		p := fmt.Sprintf("phases[%d]", i)
		if ph.ID < 1 {
			v.add(p+".id", "invalid_phase_id",
				fmt.Sprintf("%d must be a positive integer", ph.ID))
		} else if seenIDs[ph.ID] {
			v.add(p+".id", "duplicate_id", fmt.Sprintf("phase %d is declared more than once", ph.ID))
		}
		seenIDs[ph.ID] = true
		if ph.Name == "" {
			v.add(p+".name", "missing_phase_name", "phase name is required")
		}
		seenModules := make(map[string]bool, len(ph.RequiredModules))
		for j, id := range ph.RequiredModules {
			mp := fmt.Sprintf("%s.required_modules[%d]", p, j)
			if seenModules[id] {
				v.add(mp, "duplicate_entry", fmt.Sprintf("%q is listed more than once", id))
				continue
			}
			seenModules[id] = true
			v.checkModuleRef(mp, id)
		}
	}
}
