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

var (
	moduleFlag   string
	forceFlag    bool
	dryRunFlag   bool
	skipTidyFlag bool
)

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
	newCmd.Flags().BoolVar(&forceFlag, "force", false, "Overwrite existing directory without asking")
	newCmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, "Print intended operations without writing files")
	newCmd.Flags().BoolVar(&skipTidyFlag, "skip-tidy", false, "Skip running go mod tidy")
}

// dangerousTargets that should be rejected.
var dangerousTargets = []string{".", "..", "/"}

// ValidateTarget checks if the target path is safe to use.
func ValidateTarget(target string) error {
	cleaned := filepath.Clean(target)
	for _, d := range dangerousTargets {
		if cleaned == d {
			return fmt.Errorf("refusing to create project in dangerous path %q", target)
		}
	}
	if filepath.IsAbs(target) && cleaned == "/" {
		return fmt.Errorf("refusing to create project at filesystem root")
	}
	return nil
}

// PlanNewProject validates inputs and returns the resolved destination directory,
// project name, and module path. Returns an error if validation fails.
func PlanNewProject(appArg, modulePath string) (dstDir, name, module string, err error) {
	if err = ValidateTarget(appArg); err != nil {
		return
	}

	dstDir, err = filepath.Abs(filepath.Join(".", appArg))
	if err != nil {
		err = fmt.Errorf("resolve app directory: %w", err)
		return
	}

	name = filepath.Base(dstDir)
	module = modulePath
	if module == "" {
		module = name
	}
	return
}

func CreateApp(cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		fmt.Printf("%s\aappName is required%s\nUsage: glk new <appName>\n", "\x1b[31m", "\x1b[0m")
		return
	}

	dstDir, name, module, err := PlanNewProject(args[0], moduleFlag)
	if err != nil {
		fmt.Printf("%s%s%s\n", "\x1b[31m", err.Error(), "\x1b[0m")
		return
	}

	if dryRunFlag {
		fmt.Printf("[dry-run] Would create project %q in %s (module: %s)\n", name, dstDir, module)
		printTemplateFiles(dstDir)
		return
	}

	_, statErr := os.Stat(dstDir)
	if statErr == nil || os.IsExist(statErr) {
		if !forceFlag {
			fmt.Printf("%s%s%s already exists\n", "\x1b[31m", args[0], "\x1b[0m")
			fmt.Printf("Do you want to overwrite it? [y/n]: ")
			if !AskForConfirm() {
				os.Exit(255)
				return
			}
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

	fmt.Printf("Creating application %s%s%s...\n", "\x1b[32m", name, "\x1b[0m")

	if err := renderTemplates(dstDir, name, module); err != nil {
		fmt.Printf("render templates failed: %s\n", err)
		return
	}

	if !skipTidyFlag {
		if _, err := exec.Command("go", "version").Output(); err != nil {
			fmt.Printf("go not found: %s\n", err)
			return
		}

		tidyCmd := exec.Command("go", "mod", "tidy")
		tidyCmd.Dir = dstDir
		tidyCmd.Stdout = os.Stdout
		tidyCmd.Stderr = os.Stderr
		if err := tidyCmd.Run(); err != nil {
			fmt.Printf("go mod tidy failed: %s\n", err)
			return
		}
	}

	fmt.Printf("\nApplication %s%s%s created successfully.\n", "\x1b[32m", name, "\x1b[0m")
	fmt.Printf("Run: cd %s && go run .\n", args[0])
}

func renderTemplates(dstDir, name, module string) error {
	glkTpl := template.New("glk")

	return fs.WalkDir(tpl, "tpl", func(path string, dy fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel("tpl", path)
		if err != nil {
			return err
		}

		if dy.IsDir() {
			dir := filepath.Join(dstDir, rel)
			return os.MkdirAll(dir, 0755)
		}

		src, err := tpl.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()

		dst := strings.TrimSuffix(filepath.Join(dstDir, rel), ".tpl")
		dstFile, err := os.Create(dst)
		if err != nil {
			return err
		}
		defer dstFile.Close()

		fmt.Println("  +", dst)

		txt, err := io.ReadAll(src)
		if err != nil {
			return err
		}

		parser, err := glkTpl.Parse(string(bytes.TrimSpace(txt)))
		if err != nil {
			return err
		}

		return parser.Execute(dstFile, map[string]any{
			"Module": module,
			"Name":   name,
		})
	})
}

func printTemplateFiles(dstDir string) {
	fs.WalkDir(tpl, "tpl", func(path string, dy fs.DirEntry, err error) error {
		if err != nil || dy.IsDir() {
			return err
		}
		rel, _ := filepath.Rel("tpl", path)
		dst := strings.TrimSuffix(filepath.Join(dstDir, rel), ".tpl")
		fmt.Println("  [dry-run] +", dst)
		return nil
	})
}
