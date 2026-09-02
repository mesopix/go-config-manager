package configmanager

import "fmt"

// FieldType 表示配置字段期望的 JSON 类型。
type FieldType int

const (
	TypeString FieldType = iota // JSON 字符串
	TypeFloat                   // JSON 数值（float64）
	TypeBool                    // JSON 布尔值
	TypeArray                   // JSON 数组
	TypeObject                  // JSON 对象
)

// FieldDef 描述一个配置键：期望类型、是否必填，以及可选的默认值（仅在非必填时生效）。
type FieldDef struct {
	Type     FieldType
	Required bool
	Default  any // nil 表示无默认值；仅当 Required 为 false 时有意义
}

// Schema 是以配置键名为索引的字段定义集合。
// 它有意独立于 Config，使调用方自行掌控 schema 与校验生命周期。
type Schema map[string]FieldDef

// Validate checks data against the schema. For each field defined in the
// schema that exists in data, it verifies the value matches the expected
// FieldType. Missing fields and default-value filling are handled in later steps.
func (s Schema) Validate(data map[string]any) error {
	for key, def := range s {
		val, exists := data[key]
		if !exists {
			continue // missing-field check is step 4
		}
		if err := checkType(key, val, def.Type); err != nil {
			return err
		}
	}
	return nil
}

// checkType verifies that val matches the expected FieldType.
func checkType(key string, val any, expected FieldType) error {
	switch expected {
	case TypeString:
		if _, ok := val.(string); !ok {
			return fmt.Errorf("field %q: expected string, got %T", key, val)
		}
	case TypeFloat:
		if _, ok := val.(float64); !ok {
			return fmt.Errorf("field %q: expected float64, got %T", key, val)
		}
	case TypeBool:
		if _, ok := val.(bool); !ok {
			return fmt.Errorf("field %q: expected bool, got %T", key, val)
		}
	}
	return nil
}
