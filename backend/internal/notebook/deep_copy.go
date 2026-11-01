package notebook

import "reflect"

// deepCopy returns a copy of v that shares no slices, maps or pointers with it,
// so a Reader can modify what it reads (merging definitions into scenes,
// marking statements) without changing content other Readers share.
func deepCopy[T any](v T) T {
	out := copyValue(reflect.ValueOf(&v).Elem())
	return out.Interface().(T)
}

func copyValue(v reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Slice:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := 0; i < v.Len(); i++ {
			out.Index(i).Set(copyValue(v.Index(i)))
		}
		return out
	case reflect.Map:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeMapWithSize(v.Type(), v.Len())
		iter := v.MapRange()
		for iter.Next() {
			out.SetMapIndex(iter.Key(), copyValue(iter.Value()))
		}
		return out
	case reflect.Pointer:
		if v.IsNil() {
			return v
		}
		out := reflect.New(v.Type().Elem())
		out.Elem().Set(copyValue(v.Elem()))
		return out
	case reflect.Struct:
		out := reflect.New(v.Type()).Elem()
		out.Set(v) // copies every field, including unexported ones (e.g. time.Time)
		for i := 0; i < v.NumField(); i++ {
			if out.Field(i).CanSet() {
				out.Field(i).Set(copyValue(v.Field(i)))
			}
		}
		return out
	default:
		return v
	}
}
