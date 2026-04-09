package cmd

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
)

//go:embed tpl
var tpl embed.FS

var moduleFlag string

var newCmd = &cobra.Command{
	Use:   "new <appName>",
	Short: "Create a new GoLiteKit application",
	Long: `Create a new GoLiteKit application in the current directory.
The appName may include subdirectories, e.g. "glk new myorg/myapp".
Use --module to set a custom Go module path.`,
	Run: CreateApp,
}

func init() {
	newCmd.Flags().StringVar(&moduleFlag, "module", "", "Go module path (default: app directory name)")
}

func CreateApp(cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		fmt.Printf("%s\aappName is required%s\nUsage: glk new <appName>\n", "\x1b[31m", "\x1b[0m")
		return
	}

	app := args[0]
	dstDir, err := filepath.Abs(filepath.Join(".", app))
	if err != nil {
		fmt.Printf("resolve app directory failed: %s\n", err)
		return
	}

	_, err = os.Stat(dstDir)
	if err == nil || os.IsExist(err) {
		fmt.Printf("%s%s%s already exists\n", "\x1b[31m", app, "\x1b[0m")
		fmt.Printf("Do you want to overwrite it? [y/n]: ")
		if !AskForConfirm() {
			os.Exit(255)
			return
		}
		if err = os.RemoveAll(dstDir); err != nil {
			fmt.Printf("remove app directory failed: %s\n", err)
			return
		}
	}

	if err = os.MkdirAll(dstDir, 0755); err != nil {
		fmt.Printf("create app directory failed: %s\n", err)
		return
	}

	// name is the short directory base name; module is the full Go module path.
	name := filepath.Base(dstDir)
	module := moduleFlag
	if module == "" {
		module = name
	}

	fmt.Printf("Creating application %s%s%s...\n", "\x1b[32m", name, "\x1b[0m")

	glkTpl := template.New("glk")

	fs.WalkDir(tpl, "tpl", func(path string, dy fs.DirEntry, err error) error {
		if err != nil {
			fmt.Printf("walk tpl directory failed: %s\n", err)
			return err
		}

		rel, err := filepath.Rel("tpl", path)
		if err != nil {
			fmt.Printf("get relative path failed: %s\n", err)
			return err
		}

		if dy.IsDir() {
			dir := filepath.Join(dstDir, rel)
			if _, statErr := os.Stat(dir); os.IsNotExist(statErr) {
				if mkErr := os.MkdirAll(dir, 0755); mkErr != nil {
					fmt.Printf("create directory %s failed: %s\n", dir, mkErr)
					return mkErr
				}
			}
			return nil
		}

		src, err := tpl.Open(path)
		if err != nil {
			fmt.Printf("open template %s failed: %s\n", path, err)
			return err
		}
		defer src.Close()

		dst := strings.TrimSuffix(filepath.Join(dstDir, rel), ".tpl")
		dstFile, err := os.Create(dst)
		if err != nil {
			fmt.Printf("create file %s failed: %s\n", dst, err)
			return err
		}
		defer dstFile.Close()

		fmt.Println("  +", dst)

		txt, err := io.ReadAll(src)
		if err != nil {
			fmt.Printf("read template %s failed: %s\n", path, err)
			return err
		}

		parser, err := glkTpl.Parse(string(bytes.TrimSpace(txt)))
		if err != nil {
			fmt.Printf("parse template %s failed: %s\n", path, err)
			return err
		}

		if err := parser.Execute(dstFile, map[string]any{
			"Module": module,
			"Name":   name,
		}); err != nil {
			fmt.Printf("render template %s failed: %s\n", path, err)
			return err
		}

		return nil
	})

	if _, err := exec.Command("go", "version").Output(); err != nil {
		fmt.Printf("go not found: %s\n", err)
		return
	}

	if err = os.Chdir(dstDir); err != nil {
		fmt.Printf("change directory to %s failed: %s\n", dstDir, err)
		return
	}

	tidyCmd := exec.Command("go", "mod", "tidy")
	tidyCmd.Stdout = os.Stdout
	tidyCmd.Stderr = os.Stderr
	if err := tidyCmd.Run(); err != nil {
		fmt.Printf("go mod tidy failed: %s\n", err)
		return
	}

	fmt.Printf("\nApplication %s%s%s created successfully.\n", "\x1b[32m", name, "\x1b[0m")
	fmt.Printf("Run: cd %s && go run .\n", app)
}
