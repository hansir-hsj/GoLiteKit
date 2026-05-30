package cmd

import "testing"

func TestValidateTarget_RejectsDangerous(t *testing.T) {
	dangerous := []string{".", "..", "/"}
	for _, target := range dangerous {
		if err := ValidateTarget(target); err == nil {
			t.Errorf("ValidateTarget(%q) = nil, want error", target)
		}
	}
}

func TestValidateTarget_AcceptsValid(t *testing.T) {
	valid := []string{"myapp", "org/myapp", "my-project"}
	for _, target := range valid {
		if err := ValidateTarget(target); err != nil {
			t.Errorf("ValidateTarget(%q) = %v, want nil", target, err)
		}
	}
}

func TestPlanNewProject_DefaultModule(t *testing.T) {
	_, name, module, err := PlanNewProject("myapp", "")
	if err != nil {
		t.Fatalf("PlanNewProject: %v", err)
	}
	if name != "myapp" {
		t.Errorf("name = %q, want myapp", name)
	}
	if module != "myapp" {
		t.Errorf("module = %q, want myapp", module)
	}
}

func TestPlanNewProject_CustomModule(t *testing.T) {
	_, _, module, err := PlanNewProject("myapp", "github.com/org/myapp")
	if err != nil {
		t.Fatalf("PlanNewProject: %v", err)
	}
	if module != "github.com/org/myapp" {
		t.Errorf("module = %q, want github.com/org/myapp", module)
	}
}

func TestPlanNewProject_RejectsDot(t *testing.T) {
	_, _, _, err := PlanNewProject(".", "")
	if err == nil {
		t.Fatal("expected error for '.' target")
	}
}
