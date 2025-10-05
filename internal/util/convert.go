package util

import "fmt"

// ToBool converts various types to bool.
// Supports bool and string ("true"/"false") types.
func ToBool(v any) (bool, error) {
	switch b := v.(type) {
	case bool:
		return b, nil
	case string:
		if b == "true" {
			return true, nil
		}
		if b == "false" {
			return false, nil
		}
		return false, fmt.Errorf("invalid boolean string %q", b)
	default:
		return false, fmt.Errorf("invalid boolean type %T", v)
	}
}
