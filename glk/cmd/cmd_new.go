package cmd

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"sort"
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
	Args: cobra.ExactArgs(1),
	RunE: CreateApp,
}

func init() {
	newCmd.Flags().StringVar(&moduleFlag, "module", "", "Go module path (default: app directory name)")
	newCmd.Flags().BoolVar(&forceFlag, "force", false, "Overwrite generated files in an existing directory without asking")
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
	return nil
}

// PlanNewProject validates inputs and returns the resolved destination directory,
// project name, and module path. Returns an error if validation fails.
func PlanNewProject(appArg, modulePath string) (dstDir, name, module string, err error) {
	if err = ValidateTarget(appArg); err != nil {
		return
	}
	if filepath.IsAbs(appArg) {
		err = fmt.Errorf("app name must be a path relative to the current directory, got %q", appArg)
		return
	}

	cwd, err := os.Getwd()
	if err != nil {
		err = fmt.Errorf("resolve current directory: %w", err)
		return
	}
	dstDir, err = filepath.Abs(filepath.Join(cwd, appArg))
	if err != nil {
		err = fmt.Errorf("resolve app directory: %w", err)
		return
	}
	// The project must live strictly below the current directory; this also
	// rejects inputs like "../.." or "a/../../b" that slip past ValidateTarget,
	// and existing symlinks that point elsewhere.
	if !isStrictlyBelow(cwd, dstDir) || !isStrictlyBelow(resolveExisting(cwd), resolveExisting(dstDir)) {
		err = fmt.Errorf("refusing to create project outside the current directory: %q", appArg)
		return
	}

	name = filepath.Base(dstDir)
	module = modulePath
	if module == "" {
		module = name
	}
	return
}

func isStrictlyBelow(parent, path string) bool {
	rel, err := filepath.Rel(parent, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// resolveExisting resolves symlinks in the longest existing prefix of path
// and appends the rest unchanged.
func resolveExisting(path string) string {
	rest := ""
	for p := path; ; p = filepath.Dir(p) {
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(resolved, rest)
		}
		if filepath.Dir(p) == p {
			return path
		}
		rest = filepath.Join(filepath.Base(p), rest)
	}
}

// CreateApp implements "glk new". Errors are returned so the process exits non-zero.
func CreateApp(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true // arguments are valid; later errors are not usage errors
	dstDir, name, module, err := PlanNewProject(args[0], moduleFlag)
	if err != nil {
		return err
	}
	version := frameworkVersion()

	if dryRunFlag {
		fmt.Printf("[dry-run] Would create project %q in %s (module: %s)\n", name, dstDir, module)
		printTemplateFiles(dstDir)
		return nil
	}

	// Render everything before touching the filesystem so a template error
	// never leaves a half-written project behind.
	files, err := renderTemplateFiles(name, module, version)
	if err != nil {
		return fmt.Errorf("render templates: %w", err)
	}

	// Never delete an existing directory: it may hold .git or user files.
	// Only the generated template files are overwritten.
	if entries, readErr := os.ReadDir(dstDir); readErr == nil && len(entries) > 0 && !forceFlag {
		fmt.Printf("%s%s%s already exists and is not empty\n", "\x1b[31m", args[0], "\x1b[0m")
		fmt.Printf("Overwrite the generated files in it (other files are kept)? [y/n]: ")
		if !AskForConfirm() {
			return errors.New("aborted")
		}
	}

	var created []string // paths this run created, removed again on failure
	if err = mkdirAllTracked(dstDir, &created); err != nil {
		removeCreated(created)
		return fmt.Errorf("create app directory: %w", err)
	}

	fmt.Printf("Creating application %s%s%s...\n", "\x1b[32m", name, "\x1b[0m")

	if err := writeTemplateFiles(dstDir, files, &created); err != nil {
		removeCreated(created)
		return fmt.Errorf("write project files (files created by this run were removed; overwritten ones were not restored): %w", err)
	}

	if version == "" {
		fmt.Println("warning: glk was not installed from a tagged release; go.mod does not pin GoLiteKit, so go mod tidy adds the latest release, which may not have every API the generated code uses. If it fails to build, add a replace directive pointing at your GoLiteKit checkout:")
		fmt.Printf("  go mod edit -replace %s=/path/to/GoLiteKit\n", frameworkModule)
	}

	if !skipTidyFlag {
		if _, err := exec.Command("go", "version").Output(); err != nil {
			return fmt.Errorf("go not found: %w", err)
		}

		tidyCmd := exec.Command("go", "mod", "tidy")
		tidyCmd.Dir = dstDir
		tidyCmd.Stdout = os.Stdout
		tidyCmd.Stderr = os.Stderr
		if err := tidyCmd.Run(); err != nil {
			return fmt.Errorf("go mod tidy: %w", err)
		}
	}

	fmt.Printf("\nApplication %s%s%s created successfully.\n", "\x1b[32m", name, "\x1b[0m")
	fmt.Printf("Run: cd %s && go run .\n", args[0])
	return nil
}

// frameworkModule is the module the generated project depends on.
const frameworkModule = "github.com/hansir-hsj/GoLiteKit"

// frameworkVersion returns the GoLiteKit version this glk binary was built
// from (e.g. installed with go install ...@v1.3.0), so generated code matches
// the framework API. It returns "" for local or dirty builds.
func frameworkVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Path != frameworkModule {
		return ""
	}
	v := info.Main.Version
	if !strings.HasPrefix(v, "v") || strings.Contains(v, "+") {
		return ""
	}
	return v
}

// renderTemplateFiles renders the project templates in memory, keyed by
// path relative to the project root.
func renderTemplateFiles(name, module, version string) (map[string][]byte, error) {
	data := map[string]any{
		"Module":           module,
		"Name":             name,
		"FrameworkVersion": version,
	}
	files := make(map[string][]byte)

	err := fs.WalkDir(tpl, "tpl", func(path string, dy fs.DirEntry, err error) error {
		if err != nil || dy.IsDir() {
			return err
		}
		rel, err := filepath.Rel("tpl", path)
		if err != nil {
			return err
		}
		txt, err := fs.ReadFile(tpl, path)
		if err != nil {
			return err
		}
		parser, err := template.New(rel).Parse(string(bytes.TrimSpace(txt)))
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		if err := parser.Execute(&buf, data); err != nil {
			return err
		}
		files[strings.TrimSuffix(rel, ".tpl")] = buf.Bytes()
		return nil
	})
	return files, err
}

// writeTemplateFiles writes files below dstDir in path order and appends the
// files and directories it creates to created. It refuses to write through a
// symlink, which could point outside the project.
func writeTemplateFiles(dstDir string, files map[string][]byte, created *[]string) error {
	rels := make([]string, 0, len(files))
	for rel := range files {
		rels = append(rels, rel)
	}
	sort.Strings(rels)

	for _, rel := range rels {
		dst := filepath.Join(dstDir, rel)
		if err := mkdirAllTracked(filepath.Dir(dst), created); err != nil {
			return err
		}
		info, err := os.Lstat(dst)
		existed := err == nil
		switch {
		case existed && info.Mode()&fs.ModeSymlink != 0:
			return fmt.Errorf("refusing to write through symlink %s", dst)
		case !existed && !errors.Is(err, fs.ErrNotExist):
			return err
		}
		if err := os.WriteFile(dst, files[rel], 0644); err != nil {
			return err
		}
		if !existed {
			*created = append(*created, dst)
		}
		fmt.Println("  +", dst)
	}
	return nil
}

// mkdirAllTracked is os.MkdirAll that appends each directory it creates to
// created, outermost first.
func mkdirAllTracked(dir string, created *[]string) error {
	var missing []string
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Lstat(d); err == nil {
			break
		}
		missing = append(missing, d)
		if filepath.Dir(d) == d {
			break
		}
	}
	for i := len(missing) - 1; i >= 0; i-- {
		if err := os.Mkdir(missing[i], 0755); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
		*created = append(*created, missing[i])
	}
	return nil
}

// removeCreated removes paths recorded by mkdirAllTracked and
// writeTemplateFiles, innermost first. Directories that are not empty (they
// gained files from elsewhere) are left.
func removeCreated(created []string) {
	for i := len(created) - 1; i >= 0; i-- {
		_ = os.Remove(created[i])
	}
}

// renderTemplates renders the project templates into dstDir.
func renderTemplates(dstDir, name, module string) error {
	files, err := renderTemplateFiles(name, module, frameworkVersion())
	if err != nil {
		return err
	}
	var created []string
	return writeTemplateFiles(dstDir, files, &created)
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
