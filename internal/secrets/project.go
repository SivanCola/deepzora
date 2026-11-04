package secrets

import (
	"encoding/json"
	"reflect"
)

// Project masks textual fields while preserving typed counters and identities.
// Raw JSON is projected at scalar-token boundaries; binary payloads remain
// opaque. Host schemas must not turn a numeric usage counter into a string.
func Project[T any](value T) T {
	mask := Masker()
	if mask == nil {
		return value
	}
	if !reflect.ValueOf(value).IsValid() {
		return value
	}
	return projectValue(reflect.ValueOf(value), mask, 0).Interface().(T)
}

var rawJSONType = reflect.TypeFor[json.RawMessage]()

func projectValue(v reflect.Value, mask *Mask, depth int) reflect.Value {
	if !v.IsValid() {
		return v
	}
	if depth > 128 {
		// Host payloads must be acyclic. Reject a recursive tail instead of
		// returning the original subtree with unmasked strings.
		return reflect.Zero(v.Type())
	}
	if v.CanInterface() {
		if v.Type() == rawJSONType {
			return reflect.ValueOf(mask.JSON(v.Interface().(json.RawMessage)))
		}
		if err, ok := v.Interface().(error); ok && err != nil && v.Kind() == reflect.Interface {
			text := mask.String(err.Error())
			if text != err.Error() {
				return reflect.ValueOf(maskedError{original: err, text: text})
			}
			return v
		}
	}
	switch v.Kind() {
	case reflect.String:
		return reflect.ValueOf(mask.String(v.String())).Convert(v.Type())
	case reflect.Interface:
		if v.IsNil() {
			return v
		}
		child := projectValue(v.Elem(), mask, depth+1)
		out := reflect.New(v.Type()).Elem()
		out.Set(child)
		return out
	case reflect.Pointer:
		if v.IsNil() {
			return v
		}
		out := reflect.New(v.Type().Elem())
		out.Elem().Set(projectValue(v.Elem(), mask, depth+1))
		return out
	case reflect.Struct:
		out := reflect.New(v.Type()).Elem()
		out.Set(v)
		for i := range v.NumField() {
			if out.Field(i).CanSet() && v.Type().Field(i).IsExported() {
				child := projectValue(v.Field(i), mask, depth+1)
				if child.Type().AssignableTo(out.Field(i).Type()) {
					out.Field(i).Set(child)
				}
			}
		}
		return out
	case reflect.Slice:
		if v.IsNil() || v.Type().Elem().Kind() == reflect.Uint8 {
			return v
		}
		out := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := range v.Len() {
			out.Index(i).Set(projectValue(v.Index(i), mask, depth+1))
		}
		return out
	case reflect.Array:
		out := reflect.New(v.Type()).Elem()
		for i := range v.Len() {
			out.Index(i).Set(projectValue(v.Index(i), mask, depth+1))
		}
		return out
	case reflect.Map:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeMapWithSize(v.Type(), v.Len())
		iter := v.MapRange()
		for iter.Next() {
			out.SetMapIndex(projectValue(iter.Key(), mask, depth+1), projectValue(iter.Value(), mask, depth+1))
		}
		return out
	default:
		return v
	}
}
