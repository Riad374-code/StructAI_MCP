package spec

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// schemaJSON is the embedded JSON Schema for the spec format. It is
// the shape-level half of the contract; (Spec).Validate carries the
// cross-field half.
//
//go:embed spec.schema.json
var schemaJSON []byte

// schemaURL names the embedded resource for the compiler. Nothing is
// ever fetched: the schema compiles from the embedded bytes only, so
// reading or validating a spec never requires network access.
const schemaURL = "architectmcp:///spec.schema.json"

// compiledSchema compiles the embedded schema exactly once. A compile
// failure is a build defect (the schema ships inside the binary), but
// it is surfaced as an error rather than a panic so tool handlers can
// report it in-band.
var compiledSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return CompileSchema(schemaURL, schemaJSON)
})

// errMessages renders jsonschema error kinds in English, matching the
// library's own default output.
var errMessages = message.NewPrinter(language.English)

// ValidateBytes checks raw spec JSON against the embedded JSON Schema.
// Failures come back as ValidationErrors with instance paths, the same
// shape semantic validation uses, so callers (and the architect_plan
// elicitation loop) handle both validation levels uniformly.
func ValidateBytes(data []byte) error {
	schema, err := compiledSchema()
	if err != nil {
		return err
	}
	return ValidateAgainstSchema(schema, data)
}

// CompileSchema compiles an embedded JSON Schema document under the
// given resource URL. Nothing is fetched: callers pass schema bytes
// they ship inside the binary. Other packages (architect_plan's
// domain-model and architecture contracts) reuse this so every schema
// in the MCP plane compiles and reports errors the same way.
func CompileSchema(url string, schemaJSON []byte) (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		return nil, fmt.Errorf("parse embedded schema %s: %w", url, err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(url, doc); err != nil {
		return nil, fmt.Errorf("register embedded schema %s: %w", url, err)
	}
	schema, err := compiler.Compile(url)
	if err != nil {
		return nil, fmt.Errorf("compile embedded schema %s: %w", url, err)
	}
	return schema, nil
}

// ValidateAgainstSchema validates raw JSON against a compiled schema,
// flattening failures into ValidationErrors so shape problems from any
// MCP-plane document carry the same {path, rule, message} form.
func ValidateAgainstSchema(schema *jsonschema.Schema, data []byte) error {
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("document is not valid JSON: %w", err)
	}
	err = schema.Validate(instance)
	if err == nil {
		return nil
	}
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return fmt.Errorf("document failed schema validation: %w", err)
	}
	return flattenSchemaError(ve)
}

// flattenSchemaError converts the library's error tree into flat
// ValidationErrors, keeping only leaves (inner nodes restate their
// children).
func flattenSchemaError(root *jsonschema.ValidationError) ValidationErrors {
	var out ValidationErrors
	var walk func(e *jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) == 0 {
			out = append(out, ValidationError{
				Path:    instancePath(e.InstanceLocation),
				Rule:    "schema",
				Message: e.ErrorKind.LocalizedString(errMessages),
			})
			return
		}
		for _, cause := range e.Causes {
			walk(cause)
		}
	}
	walk(root)
	return out
}

// instancePath renders a JSON instance location like
// "modules[0].paths" to match semantic validation paths.
func instancePath(tokens []string) string {
	if len(tokens) == 0 {
		return "$"
	}
	var b bytes.Buffer
	for _, tok := range tokens {
		if isIndexToken(tok) {
			fmt.Fprintf(&b, "[%s]", tok)
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(tok)
	}
	return b.String()
}

func isIndexToken(tok string) bool {
	if tok == "" {
		return false
	}
	for _, r := range tok {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
