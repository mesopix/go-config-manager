package configmanager

// FieldType represents the expected JSON type of a config field.
type FieldType int

const (
	TypeString  FieldType = iota // JSON string
	TypeFloat                    // JSON number (float64)
	TypeBool                     // JSON boolean
	TypeArray                    // JSON array
	TypeObject                   // JSON object
)

// FieldDef describes one config key: its expected type, whether it is
// required, and an optional default value used when the key is missing.
type FieldDef struct {
	Type     FieldType
	Required bool
	Default  any // nil means no default; only meaningful when Required is false
}

// Schema is a set of field definitions keyed by config name.
// It is intentionally independent of Config so that callers own
// both the schema and the validation lifecycle.
type Schema map[string]FieldDef
