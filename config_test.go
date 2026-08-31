package configmanager

import (
	"path/filepath"
	"testing"
)

func TestSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	c := New(path)
	c.Set("name", "demo")
	c.Set("port", 8080)
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if name, ok := got.Get("name"); !ok || name != "demo" {
		t.Fatalf("name = %v, %v", name, ok)
	}
	if port, ok := got.Get("port"); !ok || port != float64(8080) { // JSON numbers are float64
		t.Fatalf("port = %v, %v", port, ok)
	}
}
