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

var newCmd = &cobra.Command{
	Use:   "new <appName>",
	Short: "create a new glk Application",
	Long:  `Create a new glk Application using the given appName in the current directory. The appName may include subdirectories.`,
	Run:   CreateApp,
}

func CreateApp(cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		fmt.Printf("%s\aappName is required %s \nUsage:glk new <appName> \n", "\x1b[31m", "\x1b[0m")
		return
	}
	app := args[0]
	dstDir, err := filepath.Abs(filepath.Join(".", app))
	if err != nil {
		fmt.Printf("get app directory failed: %s\n", err)
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

	err = os.MkdirAll(dstDir, 0755)
	if err != nil {
		fmt.Printf("create app directory failed: %s\n", err)
		return
	}

	name := filepath.Base(dstDir)
	fmt.Printf("Creating Application %s%s%s ing ...\n", "\x1b[31m", name, "\x1b[0m")

	glkTpl := template.New("glk")

	fs.WalkDir(tpl, "tpl", func(path string, dy fs.DirEntry, err error) error {
		if err != nil {
			fmt.Printf("walk tpl directory failed: %s\n", err)
			return err
		}

		rel, err := filepath.Rel("tpl", path)
		if err != nil {
			fmt.Printf("get tpl file path failed: %s\n", err)
			return err
		}

		if dy.IsDir() {
			dir := filepath.Join(dstDir, rel)
			_, err = os.Stat(dir)
			if err != nil && os.IsNotExist(err) {
				err = os.MkdirAll(dir, 0755)
				if err != nil {
					fmt.Printf("create directory %s failed: %s\n", dir, err)
					return err
				}
			}
			return nil
		}

		src, err := tpl.Open(path)
		if err != nil {
			fmt.Printf("open tpl file %s failed: %s\n", path, err)
			return err
		}
		defer src.Close()

		dst := filepath.Join(dstDir, rel)
		dst = strings.TrimSuffix(dst, ".tpl")
		dstFile, err := os.Create(dst)
		if err != nil {
			fmt.Printf("create file %s failed: %s\n", dst, err)
			return err
		}
		defer dstFile.Close()

		fmt.Println("creating file:", dst)

		txt, err := io.ReadAll(src)
		if err != nil {
			fmt.Printf("read tpl file %s failed: %s\n", path, err)
			return err
		}
		txt = bytes.TrimSpace(txt)
		parser, err := glkTpl.Parse(string(txt))
		if err != nil {
			fmt.Printf("parse tpl file %s failed: %s\n", path, err)
			return err
		}
		parser.Execute(dstFile, map[string]interface{}{
			"App": name,
		})

		return nil
	})

	goCmd := exec.Command("go", "version")
	if _, err := goCmd.Output(); err != nil {
		fmt.Printf("go environment not found: %s\n", err)
		return
	}

	goCmd = exec.Command("go", "mod", "tidy")
	if _, err := goCmd.Output(); err != nil {
		fmt.Printf("go mod tidy failed: %s\n", err)
		return
	}

	fmt.Printf("Application %s%s%s created successfully\n", "\x1b[31m", name, "\x1b[0m")
}
