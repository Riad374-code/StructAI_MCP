package spec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// architectDirPerm is the mode for the .architect directory.
const architectDirPerm = 0o755

// Load reads, schema-validates, and semantically validates the
// canonical spec at <projectRoot>/.architect/spec.json. It never
// touches the network: the canonical spec is local-first and any
// backend copy is only a projection.
func Load(projectRoot string) (Spec, error) {
	specPath := filepath.Join(projectRoot, filepath.FromSlash(CanonicalRelPath))
	return LoadPath(specPath)
}

// LoadPath reads and validates a spec from an explicitly resolved path.
// architect_check uses this for its optional spec_path input while Load keeps
// the canonical project-root convenience API.
func LoadPath(specPath string) (Spec, error) {
	data, err := os.ReadFile(specPath)
	if err != nil {
		return Spec{}, fmt.Errorf("read spec %s: %w", specPath, err)
	}
	if err := ValidateBytes(data); err != nil {
		return Spec{}, fmt.Errorf("spec %s failed schema validation: %w", specPath, err)
	}
	// The schema already rejects unknown fields; DisallowUnknownFields
	// is defense in depth against schema/struct drift.
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var s Spec
	if err := decoder.Decode(&s); err != nil {
		return Spec{}, fmt.Errorf("decode spec %s: %w", specPath, err)
	}
	if err := s.Validate(); err != nil {
		return Spec{}, fmt.Errorf("spec %s failed semantic validation: %w", specPath, err)
	}
	return s, nil
}

// Save validates s and writes it atomically to
// <projectRoot>/.architect/spec.json, returning the final resolved
// (normalized) spec. An invalid spec is never written: validation runs
// first, and the marshaled bytes are re-checked against the embedded
// JSON Schema so the file on disk always satisfies the published
// contract.
func Save(projectRoot string, s Spec) (Spec, error) {
	resolved := normalize(s)
	if err := resolved.Validate(); err != nil {
		return Spec{}, err
	}
	data, err := marshalDeterministic(resolved)
	if err != nil {
		return Spec{}, err
	}
	if err := ValidateBytes(data); err != nil {
		return Spec{}, fmt.Errorf("marshaled spec failed its own schema; struct/schema drift: %w", err)
	}
	specPath := filepath.Join(projectRoot, filepath.FromSlash(CanonicalRelPath))
	if err := os.MkdirAll(filepath.Dir(specPath), architectDirPerm); err != nil {
		return Spec{}, fmt.Errorf("create %s: %w", filepath.Dir(specPath), err)
	}
	if err := writeFileAtomic(specPath, data); err != nil {
		return Spec{}, err
	}
	return resolved, nil
}

// normalize returns a copy of s with every always-emitted collection
// non-nil, so semantically identical specs marshal byte-identically
// ([] never silently becomes null) regardless of how callers built
// them. The input is never mutated.
func normalize(s Spec) Spec {
	out := s
	out.Workspaces = emptyIfNil(s.Workspaces)
	out.TechStack.Required = emptyIfNil(s.TechStack.Required)
	out.TechStack.Forbidden = emptyIfNil(s.TechStack.Forbidden)
	out.TechStack.Rationales = emptyIfNil(s.TechStack.Rationales)
	out.Ignore.Paths = emptyIfNil(s.Ignore.Paths)
	out.Layers = emptyIfNil(s.Layers)
	out.LayerRules = emptyIfNil(s.LayerRules)
	out.Modules = emptyIfNil(s.Modules)
	out.Phases = normalizePhases(s.Phases)
	out.Constraints = emptyIfNil(s.Constraints)
	return out
}

// normalizePhases copies phases with non-nil required_modules, since
// that field is always emitted too.
func normalizePhases(phases []Phase) []Phase {
	out := make([]Phase, len(phases))
	for i, p := range phases {
		out[i] = p
		out[i].RequiredModules = emptyIfNil(p.RequiredModules)
	}
	return out
}

func emptyIfNil[T any](in []T) []T {
	if in == nil {
		return []T{}
	}
	return in
}

// marshalDeterministic renders the spec with the canonical on-disk
// formatting: two-space indent, struct-declaration field order
// (encoding/json guarantees this), trailing newline. Identical specs
// produce byte-identical files.
func marshalDeterministic(s Spec) ([]byte, error) {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal spec: %w", err)
	}
	return append(data, '\n'), nil
}

// writeFileAtomic writes data to path via a temp file in the same
// directory, fsync, then rename, so a crash mid-write can never leave
// a truncated spec.json behind.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".spec-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	// After a successful rename the temp file no longer exists and
	// this removal is a no-op; on any failure path it cleans up.
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename %s to %s: %w", tmpName, path, err)
	}
	return syncDir(dir)
}

// syncDir fsyncs the directory so the rename that landed in it is
// durable across a crash, completing the atomic-write pattern. Windows
// offers no directory-handle sync; the rename itself is the strongest
// guarantee available there.
func syncDir(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open %s for sync: %w", dir, err)
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("sync %s: %w", dir, err)
	}
	return nil
}
