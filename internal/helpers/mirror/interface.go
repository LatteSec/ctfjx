package mirror

import "reflect"

// NewEmpty returns an empty version of v
func NewEmpty[T any](v T) T {
	return reflect.New(reflect.TypeOf(v)).Elem().Interface().(T)
}
