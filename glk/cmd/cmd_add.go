package cmd

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
}

var addControllerCmd = &cobra.Command{
	Use:   "controller <name>",
	Short: "Generate a new controller",
	Long: `Generate a new controller file under ./controller/.
The name may use snake_case or kebab-case; it will be converted to CamelCase.

Example:
  glk add controller user_profile
  → creates controller/user_profile_controller.go with UserProfileController`,
	Run: runAddController,
}

var addMiddlewareCmd = &cobra.Command{
	Use:   "middleware <name>",
	Short: "Generate a new middleware",
	Long: `Generate a new middleware file under ./middleware/.
The name may use snake_case or kebab-case; it will be converted to CamelCase.

Example:
  glk add middleware request_id
  → creates middleware/request_id_middleware.go with RequestIdMiddleware`,
	Run: runAddMiddleware,
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

func runAddController(cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		fmt.Printf("%s\aname is required%s\nUsage: glk add controller <name>\n", "\x1b[31m", "\x1b[0m")
		return
	}
	name := args[0]
	camel := toCamelCase(name)

	outDir := "controller"
	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Printf("create directory %s failed: %s\n", outDir, err)
		return
	}

	outPath := filepath.Join(outDir, name+"_controller.go")
	if _, err := os.Stat(outPath); err == nil {
		fmt.Printf("%s%s%s already exists\n", "\x1b[31m", outPath, "\x1b[0m")
		return
	}

	renderAddTemplate("tpl_add/controller.go.tpl", outPath, map[string]any{
		"Name": camel,
	})
}

func runAddMiddleware(cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		fmt.Printf("%s\aname is required%s\nUsage: glk add middleware <name>\n", "\x1b[31m", "\x1b[0m")
		return
	}
	name := args[0]
	camel := toCamelCase(name)

	outDir := "middleware"
	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Printf("create directory %s failed: %s\n", outDir, err)
		return
	}

	outPath := filepath.Join(outDir, name+"_middleware.go")
	if _, err := os.Stat(outPath); err == nil {
		fmt.Printf("%s%s%s already exists\n", "\x1b[31m", outPath, "\x1b[0m")
		return
	}

	renderAddTemplate("tpl_add/middleware.go.tpl", outPath, map[string]any{
		"Name": camel,
	})
}

// renderAddTemplate reads a tpl_add template, renders it with data, and writes the result to outPath.
func renderAddTemplate(tplPath, outPath string, data map[string]any) {
	src, err := tplAdd.Open(tplPath)
	if err != nil {
		fmt.Printf("open template %s failed: %s\n", tplPath, err)
		return
	}
	defer src.Close()

	content, err := io.ReadAll(src)
	if err != nil {
		fmt.Printf("read template %s failed: %s\n", tplPath, err)
		return
	}

	t, err := template.New("").Parse(string(bytes.TrimSpace(content)))
	if err != nil {
		fmt.Printf("parse template %s failed: %s\n", tplPath, err)
		return
	}

	outFile, err := os.Create(outPath)
	if err != nil {
		fmt.Printf("create file %s failed: %s\n", outPath, err)
		return
	}
	defer outFile.Close()

	if err := t.Execute(outFile, data); err != nil {
		fmt.Printf("render template %s failed: %s\n", tplPath, err)
		return
	}

	fmt.Printf("created: %s\n", outPath)
}
