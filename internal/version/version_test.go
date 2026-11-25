package version

import "testing"

func TestGet(t *testing.T) {
	old := Version
	t.Cleanup(func() { Version = old })

	Version = "v9.9.9-test"
	if got := Get().Version; got != "v9.9.9-test" {
		t.Fatalf("version = %q, want v9.9.9-test", got)
	}

	// без -X версия всё равно должна быть непустой
	Version = ""
	if got := Get().Version; got == "" {
		t.Fatal("fallback version is empty")
	}
	if got := String(); got == "" {
		t.Fatal("String() is empty")
	}
}
