package mirror

import "reflect"

// Fresh returns a new zeroed instance of T.
// If T is a pointer type, it allocates the pointed-to value and returns T itself.
// If T is a value type, it returns a pointer to a new zeroed value.
func Fresh[T any]() any {
	var zero T
	typ := reflect.TypeOf(zero)

	var a reflect.Type

	if typ.Kind() == reflect.Ptr {
		a = typ.Elem() // alloc underlying
	}

	return reflect.New(a).Interface() // type: *T
}
