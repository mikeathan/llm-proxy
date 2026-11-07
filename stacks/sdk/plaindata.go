package stacksdk

import (
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
)

var (
	jsonMarshaler = reflect.TypeFor[json.Marshaler]()
	textMarshaler = reflect.TypeFor[encoding.TextMarshaler]()
)

// CheckPlainData reports why t cannot cross a port: it holds a function, a channel, an interface, an unsafe pointer, a pointer, or a
// map keyed by something other than a string. A type that marshals itself (time.Time, json.RawMessage) is a leaf. Every type that
// crosses a port must pass (Constitution IV.6).
func CheckPlainData(t reflect.Type) error {
	return checkPlain(t, t.String(), map[reflect.Type]bool{})
}

func checkPlain(t reflect.Type, path string, seen map[reflect.Type]bool) error {
	if t.Implements(jsonMarshaler) || t.Implements(textMarshaler) || reflect.PointerTo(t).Implements(jsonMarshaler) || reflect.PointerTo(t).Implements(textMarshaler) {
		return nil
	}
	switch t.Kind() {
	case reflect.Func, reflect.Chan, reflect.UnsafePointer, reflect.Interface, reflect.Pointer:
		return fmt.Errorf("%s is a %s, which is not plain data", path, t.Kind())
	case reflect.Slice, reflect.Array:
		return checkPlain(t.Elem(), path+"[]", seen)
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return fmt.Errorf("%s is a map with a %s key; keys must be strings", path, t.Key().Kind())
		}
		return checkPlain(t.Elem(), path+"{}", seen)
	case reflect.Struct:
		return checkStruct(t, path, seen)
	}
	return nil
}

func checkStruct(t reflect.Type, path string, seen map[reflect.Type]bool) error {
	if seen[t] {
		return nil
	}
	seen[t] = true
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() || f.Tag.Get("json") == "-" {
			continue
		}
		if err := checkPlain(f.Type, path+"."+f.Name, seen); err != nil {
			return err
		}
	}
	return nil
}
