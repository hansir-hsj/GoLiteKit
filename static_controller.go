package golitekit

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
)

type StaticController struct {
	BaseController
	Path string
}

func (c *StaticController) Serve(ctx context.Context) error {
	return c.Handle(ctx)
}

func (c *StaticController) Handle(ctx context.Context) error {
	f, err := os.Open(c.Path)
	if err != nil {
		return err
	}
	defer f.Close()

	d, err := f.Stat()
	if err != nil {
		return err
	}

	if d.IsDir() {
		raw, err := c.HandleDir(f)
		if err != nil {
			return err
		}
		c.gcx.ServeHTML(raw)
		return nil
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return err
	}

	ext := filepath.Ext(c.Path)
	c.gcx.ServeFile(ext, data)

	return nil
}

func (c *StaticController) HandleDir(f http.File) (string, error) {
	dirs, err := f.Readdir(-1)
	if err != nil {
		return "", err
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name() < dirs[j].Name() })

	raw := "<pre>\n"
	for _, d := range dirs {
		name := d.Name()
		if d.IsDir() {
			name += "/"
		}
		url := url.URL{Path: name}
		raw += fmt.Sprintf("<a href=\"%s\">%s</a>\n", url.String(), name)
	}
	raw += "</pre>\n"

	return raw, nil
}
