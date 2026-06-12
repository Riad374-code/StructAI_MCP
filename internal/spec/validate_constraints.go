package spec

import (
	"fmt"
	"strings"
)

// constraints validates the explicit constraint list. Field rules are
// driven entirely by the kindSpecs table so the closed kind set, the
// engine check mapping, and validation can never disagree.
func (v *validator) constraints(s Spec) {
	for i, c := range s.Constraints {
		p := fmt.Sprintf("constraints[%d]", i)
		v.checkID(p+".id", c.ID, v.ruleIDs)
		ks, known := kindSpecs[c.Kind]
		if !known {
			v.add(p+".kind", "unknown_constraint_kind",
				fmt.Sprintf("%q has no engine check mapping; known kinds: %s", c.Kind, knownKindList()))
			continue
		}
		v.constraintFields(p, c, ks)
		v.constraintRefs(p, c)
		if !validSeverity(c.Severity) {
			v.add(p+".severity", "invalid_severity",
				fmt.Sprintf("%q is not one of critical, high, medium, low", c.Severity))
		}
	}
}

// constraintFields enforces the per-kind field contract: every
// required field set, no field outside required+optional set. A
// misplaced field means the author intended a rule the engine would
// silently not enforce, so it is an error, not a warning.
func (v *validator) constraintFields(p string, c Constraint, ks kindSpec) {
	allowed := make(map[constraintField]bool, len(ks.required)+len(ks.optional))
	for _, f := range ks.required {
		allowed[f] = true
	}
	for _, f := range ks.optional {
		allowed[f] = true
	}
	set := make(map[constraintField]bool, len(unionFields))
	for _, uf := range unionFields {
		if !uf.isSet(c) {
			continue
		}
		set[uf.name] = true
		if !allowed[uf.name] {
			v.add(fmt.Sprintf("%s.%s", p, uf.name), "unexpected_field",
				fmt.Sprintf("%q does not apply to kind %q", uf.name, c.Kind))
		}
	}
	for _, f := range ks.required {
		if !set[f] {
			v.add(fmt.Sprintf("%s.%s", p, f), "missing_field",
				fmt.Sprintf("kind %q requires %q", c.Kind, f))
		}
	}
}

// constraintRefs resolves every set reference field against the
// declared module/layer sets and rejects self-edges and non-positive
// thresholds.
func (v *validator) constraintRefs(p string, c Constraint) {
	if c.FromModule != "" {
		v.checkModuleRef(p+".from_module", c.FromModule)
	}
	if c.ToModule != "" {
		v.checkModuleRef(p+".to_module", c.ToModule)
	}
	if c.Module != "" {
		v.checkModuleRef(p+".module", c.Module)
	}
	if c.FromLayer != "" {
		v.checkLayerRef(p+".from_layer", c.FromLayer)
	}
	if c.ToLayer != "" {
		v.checkLayerRef(p+".to_layer", c.ToLayer)
	}
	if c.FromModule != "" && c.FromModule == c.ToModule {
		v.add(p, "self_reference", "a constraint cannot point a module at itself")
	}
	if c.FromLayer != "" && c.FromLayer == c.ToLayer {
		v.add(p, "self_reference", "a constraint cannot point a layer at itself")
	}
	// A zero threshold on coupling_threshold already reports as
	// missing_field; only negatives need flagging here.
	if c.Threshold < 0 {
		v.add(p+".threshold", "invalid_threshold",
			fmt.Sprintf("%d must be a positive integer", c.Threshold))
	}
}

// knownKindList renders the closed kind set in its fixed declaration
// order for deterministic error messages.
func knownKindList() string {
	names := make([]string, len(constraintKinds))
	for i, k := range constraintKinds {
		names[i] = string(k)
	}
	return strings.Join(names, ", ")
}
