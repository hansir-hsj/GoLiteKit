package golitekit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type bindBase struct {
	TraceID string `form:"trace_id"`
}

type bindReq struct {
	bindBase
	*BindExtra // exported: an unexported embedded pointer cannot be allocated

	Name     string   `json:"name,omitempty"`
	Secret   string   `json:"-"`
	Skipped  string   `form:"-" json:"skipped"`
	Small    int8     `form:"small"`
	Ratio    float32  `form:"ratio"`
	Flag     bool     `form:"flag"`
	Tags     []string `form:"tag"`
	IDs      []int64  `form:"id"`
	Opt      *uint16  `form:"opt"`
	Fallback string
}

type BindExtra struct {
	Region string `form:"region"`
}

func bindFromForm(t *testing.T, contentType string, form url.Values) (*bindReq, error) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	ctx := withContext(req.Context())
	req = req.WithContext(ctx)
	GetContext(ctx).setContextOptions(withRequest(req), withResponseWriter(rec))

	c := &BaseControllerOf[bindReq]{}
	if err := c.Init(req.Context()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	err := c.ParseRequest(context.Background())
	return &c.Request, err
}

func TestFormBindingFields(t *testing.T) {
	form := url.Values{
		"trace_id": {"t-1"},
		"region":   {"eu"},
		"name":     {"alice"},
		"Secret":   {"leak"},
		"skipped":  {"x"},
		"small":    {"-128"},
		"ratio":    {"0.5"},
		"flag":     {"TRUE"},
		"tag":      {"a", "b"},
		"id":       {"1", "2", "3"},
		"opt":      {"65535"},
		"Fallback": {"fb"},
	}
	// Mixed-case media type with parameters must still be treated as a form.
	got, err := bindFromForm(t, "Application/X-WWW-Form-Urlencoded; charset=UTF-8", form)
	if err != nil {
		t.Fatalf("ParseRequest: %v", err)
	}

	if got.TraceID != "t-1" || got.BindExtra == nil || got.Region != "eu" {
		t.Errorf("embedded fields not bound: %+v %+v", got.bindBase, got.BindExtra)
	}
	if got.Name != "alice" {
		t.Errorf("json:\"name,omitempty\" not matched: %q", got.Name)
	}
	if got.Secret != "" || got.Skipped != "" {
		t.Errorf("\"-\" fields must be skipped: Secret=%q Skipped=%q", got.Secret, got.Skipped)
	}
	if got.Small != -128 || got.Ratio != 0.5 || !got.Flag || got.Fallback != "fb" {
		t.Errorf("scalars = %d %v %v %q", got.Small, got.Ratio, got.Flag, got.Fallback)
	}
	if strings.Join(got.Tags, ",") != "a,b" || len(got.IDs) != 3 || got.IDs[2] != 3 {
		t.Errorf("slices = %v %v", got.Tags, got.IDs)
	}
	if got.Opt == nil || *got.Opt != 65535 {
		t.Errorf("Opt = %v", got.Opt)
	}
}

func TestFormBindingRejectsInvalidValues(t *testing.T) {
	cases := map[string]url.Values{
		"int8 overflow":   {"small": {"300"}},
		"uint16 overflow": {"opt": {"65536"}},
		"invalid bool":    {"flag": {"yes"}},
		"bad slice item":  {"id": {"1", "x"}},
	}
	for name, form := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := bindFromForm(t, mediaTypeForm, form)
			appErr := WrapError(err, 0)
			if appErr == nil || appErr.Code != http.StatusBadRequest {
				t.Fatalf("err = %v, want 400", err)
			}
		})
	}
}

// Browsers submit "on" for a checked checkbox and "" for empty inputs.
func TestFormBindingAcceptsBrowserFormValues(t *testing.T) {
	got, err := bindFromForm(t, mediaTypeForm, url.Values{
		"flag":  {"on"},
		"small": {""},
		"opt":   {""},
		"name":  {""},
	})
	if err != nil {
		t.Fatalf("ParseRequest: %v", err)
	}
	if !got.Flag || got.Small != 0 || got.Opt != nil {
		t.Errorf("Flag=%v Small=%d Opt=%v", got.Flag, got.Small, got.Opt)
	}
}

func TestQueryBoolInvalidReturnsDefault(t *testing.T) {
	req, _, _ := makeRequest(http.MethodGet, "/?a=yes&b=T", nil, "")
	c := &BaseController{}
	if err := c.Init(req.Context()); err != nil {
		t.Fatal(err)
	}
	if !c.QueryBool("a", true) || c.QueryBool("a", false) {
		t.Error("invalid bool should return the default")
	}
	if !c.QueryBool("b", false) {
		t.Error("\"T\" should parse as true")
	}
}
