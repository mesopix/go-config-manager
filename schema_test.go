package appconfig

import "testing"

// ---------- Check 五种状态覆盖 ----------

func TestCheck_valid(t *testing.T) {
	s := Schema{"name": {Type: TypeString, Required: true}}
	data := map[string]any{"name": "demo"}
	if got := s.Check(data); got != Valid {
		t.Errorf("Check = %d, want Valid", got)
	}
}

func TestCheck_missingDefaults(t *testing.T) {
	s := Schema{"port": {Type: TypeFloat, Default: float64(8080)}}
	data := map[string]any{}
	if got := s.Check(data); got != MissingDefaults {
		t.Errorf("Check = %d, want MissingDefaults", got)
	}
}

func TestCheck_extraFields(t *testing.T) {
	s := Schema{"name": {Type: TypeString, Required: true}}
	data := map[string]any{"name": "demo", "extra": "surprise"}
	if got := s.Check(data); got != ExtraFields {
		t.Errorf("Check = %d, want ExtraFields", got)
	}
}

func TestCheck_missingAndExtra(t *testing.T) {
	s := Schema{
		"name": {Type: TypeString, Required: true},
		"port": {Type: TypeFloat, Default: float64(8080)},
	}
	data := map[string]any{"name": "demo", "extra": "surprise"}
	if got := s.Check(data); got != MissingAndExtra {
		t.Errorf("Check = %d, want MissingAndExtra", got)
	}
}

func TestCheck_invalid_requiredMissing(t *testing.T) {
	s := Schema{"name": {Type: TypeString, Required: true}}
	data := map[string]any{}
	if got := s.Check(data); got != Invalid {
		t.Errorf("Check = %d, want Invalid (required missing)", got)
	}
}

func TestCheck_invalid_typeMismatch(t *testing.T) {
	s := Schema{"port": {Type: TypeFloat, Required: true}}
	data := map[string]any{"port": "not-a-number"}
	if got := s.Check(data); got != Invalid {
		t.Errorf("Check = %d, want Invalid (type mismatch)", got)
	}
}

// ---------- Normalize 三种可转换状态 + 收敛验证 ----------

func TestNormalize_missingDefaults(t *testing.T) {
	s := Schema{"port": {Type: TypeFloat, Default: float64(8080)}}
	data := map[string]any{}

	result, err := s.Normalize(data)
	if err != nil {
		t.Fatalf("Normalize: unexpected error: %v", err)
	}
	if port, ok := result["port"]; !ok || port != float64(8080) {
		t.Errorf("port = %v, %v; want 8080, true", port, ok)
	}
	// 原 data 未被修改
	if _, exists := data["port"]; exists {
		t.Error("original data was modified")
	}
	// 收敛验证：Normalize 后 Check 应为 Valid
	if got := s.Check(result); got != Valid {
		t.Errorf("after Normalize: Check = %d, want Valid", got)
	}
}

func TestNormalize_extraFields(t *testing.T) {
	s := Schema{"name": {Type: TypeString, Required: true}}
	data := map[string]any{"name": "demo", "extra": "surprise"}

	result, err := s.Normalize(data)
	if err != nil {
		t.Fatalf("Normalize: unexpected error: %v", err)
	}
	if _, exists := result["extra"]; exists {
		t.Error("extra field should be removed")
	}
	if name, ok := result["name"]; !ok || name != "demo" {
		t.Errorf("name = %v, %v; want demo, true", name, ok)
	}
	if got := s.Check(result); got != Valid {
		t.Errorf("after Normalize: Check = %d, want Valid", got)
	}
}

func TestNormalize_missingAndExtra(t *testing.T) {
	s := Schema{
		"name": {Type: TypeString, Required: true},
		"port": {Type: TypeFloat, Default: float64(8080)},
	}
	data := map[string]any{"name": "demo", "extra": "surprise"}

	result, err := s.Normalize(data)
	if err != nil {
		t.Fatalf("Normalize: unexpected error: %v", err)
	}
	if _, exists := result["extra"]; exists {
		t.Error("extra field should be removed")
	}
	if port, ok := result["port"]; !ok || port != float64(8080) {
		t.Errorf("port = %v, %v; want 8080, true", port, ok)
	}
	if got := s.Check(result); got != Valid {
		t.Errorf("after Normalize: Check = %d, want Valid", got)
	}
}

// ---------- Normalize 不可转换状态应报错 ----------

func TestNormalize_valid_returnsError(t *testing.T) {
	s := Schema{"name": {Type: TypeString, Required: true}}
	data := map[string]any{"name": "demo"}

	_, err := s.Normalize(data)
	if err == nil {
		t.Fatal("Normalize on Valid data: expected error, got nil")
	}
}

func TestNormalize_invalid_returnsError(t *testing.T) {
	s := Schema{"port": {Type: TypeFloat, Required: true}}
	data := map[string]any{"port": "wrong"}

	_, err := s.Normalize(data)
	if err == nil {
		t.Fatal("Normalize on Invalid data: expected error, got nil")
	}
}
