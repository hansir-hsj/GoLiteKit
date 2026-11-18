package cmd

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
)

//go:embed tpl_add
var tplAdd embed.FS

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a component to the current project",
	Long:  "Add a new controller or middleware file to the current GoLiteKit project.",
	// Without RunE cobra prints help and exits 0 for a mistyped component.
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return fmt.Errorf("unknown component %q: want controller or middleware", args[0])
		}
		return cmd.Help()
	},
}

var addControllerCmd = &cobra.Command{
	Use:   "controller <name>",
	Short: "Generate a new controller",
	Long: `Generate a new controller file under ./controller/.
The name may use snake_case or kebab-case; it will be converted to CamelCase.

Example:
  glk add controller user_profile
  → creates controller/user_profile_controller.go with UserProfileController`,
	Args: cobra.ExactArgs(1),
	RunE: runAddController,
}

var addMiddlewareCmd = &cobra.Command{
	Use:   "middleware <name>",
	Short: "Generate a new middleware",
	Long: `Generate a new middleware file under ./middleware/.
The name may use snake_case or kebab-case; it will be converted to CamelCase.

Example:
  glk add middleware request_id
  → creates middleware/request_id_middleware.go with RequestIdMiddleware`,
	Args: cobra.ExactArgs(1),
	RunE: runAddMiddleware,
}

func init() {
	addCmd.AddCommand(addControllerCmd)
	addCmd.AddCommand(addMiddlewareCmd)
}

// toCamelCase converts snake_case or kebab-case to CamelCase.
func toCamelCase(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-'
	})
	var b strings.Builder
	for _, p := range parts {
		if len(p) > 0 {
			b.WriteString(strings.ToUpper(p[:1]) + p[1:])
		}
	}
	return b.String()
}

// componentNamePattern limits names to a single path segment that converts to
// a Go identifier, so the output file stays inside its directory.
var componentNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

// validateComponentName checks a controller/middleware name and returns its CamelCase form.
func validateComponentName(name string) (string, error) {
	if !componentNamePattern.MatchString(name) {
		return "", fmt.Errorf("invalid name %q: must start with a letter and contain only letters, digits, '_' or '-'", name)
	}
	camel := toCamelCase(name)
	if !token.IsIdentifier(camel) {
		return "", fmt.Errorf("invalid name %q: %q is not a valid Go identifier", name, camel)
	}
	return camel, nil
}

func runAddController(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true
	return addComponent(args[0], "controller", "_controller.go", "tpl_add/controller.go.tpl")
}

func runAddMiddleware(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true
	return addComponent(args[0], "middleware", "_middleware.go", "tpl_add/middleware.go.tpl")
}

func addComponent(name, outDir, suffix, tplPath string) error {
	camel, err := validateComponentName(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("create directory %s: %w", outDir, err)
	}
	outPath := filepath.Join(outDir, name+suffix)
	if err := renderAddTemplate(tplPath, outPath, map[string]any{"Name": camel}); err != nil {
		return err
	}
	fmt.Printf("created: %s\n", outPath)
	return nil
}

// renderAddTemplate renders a tpl_add template with data and writes it to
// outPath. It fails if outPath already exists, and renders fully before
// creating the file so errors never leave a partial file.
func renderAddTemplate(tplPath, outPath string, data map[string]any) error {
	content, err := fs.ReadFile(tplAdd, tplPath)
	if err != nil {
		return fmt.Errorf("read template %s: %w", tplPath, err)
	}
	t, err := template.New("").Parse(string(bytes.TrimSpace(content)))
	if err != nil {
		return fmt.Errorf("parse template %s: %w", tplPath, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return fmt.Errorf("render template %s: %w", tplPath, err)
	}

	outFile, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("%s already exists", outPath)
		}
		return fmt.Errorf("create file %s: %w", outPath, err)
	}
	if _, err := outFile.Write(buf.Bytes()); err != nil {
		outFile.Close()
		return fmt.Errorf("write file %s: %w", outPath, err)
	}
	return outFile.Close()
}
