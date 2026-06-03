package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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

func repoRoot(t *testing.T) string {
	t.Helper()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	return filepath.Clean(filepath.Join(cwd, "..", ".."))
}

func writeTempModule(t *testing.T, dir string) {
	t.Helper()

	mod := "module example.com/generated\n\n" +
		"go 1.23\n\n" +
		"require github.com/hansir-hsj/GoLiteKit v0.0.0\n\n" +
		"replace github.com/hansir-hsj/GoLiteKit => " + repoRoot(t) + "\n"

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
}

func runGoTest(t *testing.T, dir string) string {
	t.Helper()

	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = dir
	if out, err := tidy.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy failed: %v\n%s", err, out)
	}

	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go test ./... failed: %v\n%s", err, out)
	}
	return string(out)
}

func TestRenderAddControllerTemplate_Compiles(t *testing.T) {
	dir := t.TempDir()
	writeTempModule(t, dir)

	outDir := filepath.Join(dir, "controller")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("mkdir controller: %v", err)
	}

	outPath := filepath.Join(outDir, "user_controller.go")
	renderAddTemplate("tpl_add/controller.go.tpl", outPath, map[string]any{"Name": "User"})

	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read rendered controller: %v", err)
	}
	if strings.Contains(string(content), "BaseController[") {
		t.Fatalf("rendered controller uses obsolete generic BaseController syntax:\n%s", content)
	}
	if !strings.Contains(string(content), "kit.BaseController") {
		t.Fatalf("rendered controller should embed kit.BaseController:\n%s", content)
	}

	runGoTest(t, dir)
}

func TestRenderAddMiddlewareTemplate_CompilesAsGoLiteKitMiddleware(t *testing.T) {
	dir := t.TempDir()
	writeTempModule(t, dir)

	outDir := filepath.Join(dir, "middleware")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("mkdir middleware: %v", err)
	}

	outPath := filepath.Join(outDir, "request_id_middleware.go")
	renderAddTemplate("tpl_add/middleware.go.tpl", outPath, map[string]any{"Name": "RequestId"})

	usage := `package generated

import (
	"testing"

	kit "github.com/hansir-hsj/GoLiteKit"
	"example.com/generated/middleware"
)

func TestGeneratedMiddlewareHasFrameworkType(t *testing.T) {
	var _ kit.Middleware = middleware.RequestIdMiddleware
}
`
	if err := os.WriteFile(filepath.Join(dir, "generated_middleware_test.go"), []byte(usage), 0644); err != nil {
		t.Fatalf("write usage test: %v", err)
	}

	runGoTest(t, dir)
}

func TestRenderTemplates_NewProjectCompiles(t *testing.T) {
	dir := t.TempDir()
	module := "example.com/generated-app"

	if err := renderTemplates(dir, "generated-app", module); err != nil {
		t.Fatalf("renderTemplates: %v", err)
	}

	goModPath := filepath.Join(dir, "go.mod")
	goMod, err := os.ReadFile(goModPath)
	if err != nil {
		t.Fatalf("read generated go.mod: %v", err)
	}

	goModText := string(goMod) +
		"\nrequire github.com/hansir-hsj/GoLiteKit v0.0.0\n" +
		"replace github.com/hansir-hsj/GoLiteKit => " + repoRoot(t) + "\n"
	if err := os.WriteFile(goModPath, []byte(goModText), 0644); err != nil {
		t.Fatalf("rewrite generated go.mod: %v", err)
	}

	controllerPath := filepath.Join(dir, "controller", "hello_controller.go")
	controller, err := os.ReadFile(controllerPath)
	if err != nil {
		t.Fatalf("read generated controller: %v", err)
	}
	if strings.Contains(string(controller), "BaseController[") {
		t.Fatalf("generated controller uses obsolete generic BaseController syntax:\n%s", controller)
	}

	runGoTest(t, dir)
}
