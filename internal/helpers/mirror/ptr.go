package mirror

import (
	"errors"
	"reflect"
)

var (
	ErrNotPointer         = errors.New("not a pointer")
	ErrNilPointer         = errors.New("nil pointer")
	ErrInvalidPointerKind = errors.New("invalid pointer")
)

func IsStructPointer(v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer {
		return ErrNotPointer
	}
	if rv.IsNil() {
		return ErrNilPointer
	}
	if rv.Elem().Kind() != reflect.Struct {
		return ErrInvalidPointerKind
	}
	return nil
}
