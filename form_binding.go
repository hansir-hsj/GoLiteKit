package golitekit

import (
	"fmt"
	"mime"
	"net/http"
	"reflect"
	"strconv"
	"strings"
)

const (
	mediaTypeForm      = "application/x-www-form-urlencoded"
	mediaTypeMultipart = "multipart/form-data"
)

// mediaType returns the request's lowercased media type without parameters,
// or "" if Content-Type is missing or malformed.
func mediaType(r *http.Request) string {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return ""
	}
	return mt // ParseMediaType lowercases it
}

// formFieldName returns the form key for a struct field: the name part of the
// "form" tag, else of the "json" tag, else the field name. skip is true for
// tags set to "-".
func formFieldName(f reflect.StructField) (name string, skip bool) {
	for _, key := range []string{"form", "json"} {
		tag, ok := f.Tag.Lookup(key)
		if !ok {
			continue
		}
		name, _, _ = strings.Cut(tag, ",")
		if name == "-" {
			return "", true
		}
		if name != "" {
			return name, false
		}
	}
	return f.Name, false
}

// bindForm sets the fields of the struct pointed to by dst from form values.
// Anonymous embedded structs (or pointers to them) are flattened like in
// encoding/json; slice fields take every value of a repeated key.
func bindForm(dst any, form map[string][]string) error {
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Pointer || v.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("dst must be a struct pointer")
	}
	return bindStruct(v.Elem(), form)
}

func bindStruct(v reflect.Value, form map[string][]string) error {
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		ft := t.Field(i)
		field := v.Field(i)

		if ft.Anonymous && ft.Tag.Get("form") == "" && ft.Tag.Get("json") == "" {
			if err := bindEmbedded(field, form); err != nil {
				return err
			}
			continue
		}
		if !field.CanSet() {
			continue
		}

		name, skip := formFieldName(ft)
		if skip {
			continue
		}
		values := form[name]
		if len(values) == 0 {
			continue
		}

		var err error
		if field.Kind() == reflect.Slice && field.Type().Elem().Kind() != reflect.Uint8 {
			err = setSliceValue(field, values)
		} else {
			err = setFieldValue(field, values[0])
		}
		if err != nil {
			return ErrBadRequest(fmt.Sprintf("Invalid value for field %q", name), err)
		}
	}
	return nil
}

// bindEmbedded flattens an anonymous embedded struct or *struct field.
func bindEmbedded(field reflect.Value, form map[string][]string) error {
	switch {
	case field.Kind() == reflect.Struct:
		return bindStruct(field, form)
	case field.Kind() == reflect.Pointer && field.Type().Elem().Kind() == reflect.Struct:
		if field.IsNil() {
			if !field.CanSet() {
				return nil // unexported embedded pointer: cannot allocate it
			}
			field.Set(reflect.New(field.Type().Elem()))
		}
		return bindStruct(field.Elem(), form)
	}
	return nil
}

func setSliceValue(field reflect.Value, values []string) error {
	slice := reflect.MakeSlice(field.Type(), len(values), len(values))
	for i, s := range values {
		if err := setFieldValue(slice.Index(i), s); err != nil {
			return err
		}
	}
	field.Set(slice)
	return nil
}

// setFieldValue sets a scalar field (or pointer to one) from a string, using
// the field's bit size. Empty values leave non-string fields unchanged.
func setFieldValue(field reflect.Value, value string) error {
	if value == "" && !isStringField(field) {
		return nil
	}
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(value, 10, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetInt(n)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(value, 10, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetUint(n)

	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(value, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetFloat(f)

	case reflect.Bool:
		b, err := parseBool(value)
		if err != nil {
			return err
		}
		field.SetBool(b)

	case reflect.Pointer:
		// Optional fields declared as *string, *int64, *bool, etc.
		ptr := reflect.New(field.Type().Elem())
		if err := setFieldValue(ptr.Elem(), value); err != nil {
			return err
		}
		field.Set(ptr)

	default:
		return fmt.Errorf("unsupported field type: %v", field.Type())
	}

	return nil
}

// isStringField reports whether field is a string or a pointer chain to one.
func isStringField(field reflect.Value) bool {
	t := field.Type()
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Kind() == reflect.String
}
