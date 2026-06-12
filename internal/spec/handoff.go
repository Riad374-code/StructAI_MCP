package spec

// BackendHandoff is the projection of a spec the product-plane backend
// may receive for dashboard display. This file defines the contract
// only — no backend client lives in this repository, mirroring is
// optional and idempotent, and the local .architect/spec.json remains
// the source of truth.
//
// CreatedAt/UpdatedAt are RFC 3339 timestamps supplied by the tool
// handler envelope. They are deliberately plain parameters: this
// package, like the engine, never reads the wall clock.
type BackendHandoff struct {
	SpecID        string `json:"spec_id"`
	SpecVersion   int    `json:"spec_version"`
	SchemaVersion string `json:"schema_version"`

	Project     Project      `json:"project"`
	Phases      []Phase      `json:"phases"`
	Modules     []Module     `json:"modules"`
	Constraints []Constraint `json:"constraints"`
	TechStack   TechStack    `json:"tech_stack"`

	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// NewBackendHandoff projects s into the backend contract. The
// projection is lossy by design: workspaces, ignore patterns, layers,
// and layer rules stay local because the dashboard has no use for
// them, and anything the backend holds must be reconstructible from
// the canonical file.
func NewBackendHandoff(s Spec, createdAt, updatedAt string) BackendHandoff {
	resolved := normalize(s)
	return BackendHandoff{
		SpecID:        resolved.SpecID,
		SpecVersion:   resolved.SpecVersion,
		SchemaVersion: resolved.SchemaVersion,
		Project:       resolved.Project,
		Phases:        resolved.Phases,
		Modules:       resolved.Modules,
		Constraints:   resolved.Constraints,
		TechStack:     resolved.TechStack,
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
	}
}
