package cloner

import (
	"reflect"
	"strings"
)

const (
	fieldTagName = "clone"

	fieldTagValueSkip       = "skip"
	fieldTagValueSkipAlias  = "-"
	fieldTagValueShadowCopy = "shadow"
)

type Config struct {
	disallowTypes []reflect.Type
}

type Cloner[T any] struct {
	*Config
}

func (cloner *Cloner[T]) Clone(src *T) *T {
	var nilPtr *T

	if src == nilPtr {
		return nil
	}

	dst := new(T)
	val := cloner.cloneValue(reflect.ValueOf(src))

	reflect.ValueOf(dst).Elem().Set(val.Elem())

	return dst
}

func (cloner *Cloner[T]) cloneValue(src reflect.Value) reflect.Value {
	for i := range cloner.disallowTypes {
		if src.Type() == cloner.disallowTypes[i] {
			return src
		}
	}

	if !src.IsValid() {
		return src
	}

	// Look up the corresponding clone function.
	switch src.Kind() {
	case reflect.Bool:
		return cloner.cloneBool(src)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return cloner.cloneInt(src)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return cloner.cloneUint(src)
	case reflect.Float32, reflect.Float64:
		return cloner.cloneFloat(src)
	case reflect.String:
		return cloner.cloneString(src)
	case reflect.Slice:
		return cloner.cloneSlice(src)
	case reflect.Array:
		return cloner.cloneArray(src)
	case reflect.Map:
		return cloner.cloneMap(src)
	case reflect.Ptr, reflect.UnsafePointer:
		return cloner.clonePointer(src)
	case reflect.Struct:
		return cloner.cloneStruct(src)
	case reflect.Invalid, reflect.Uintptr, reflect.Complex64, reflect.Complex128, reflect.Chan, reflect.Func, reflect.Interface:
	}

	return src
}

func (cloner *Cloner[T]) cloneInt(src reflect.Value) reflect.Value {
	dst := reflect.New(src.Type()).Elem()
	dst.SetInt(src.Int())

	return dst
}

func (cloner *Cloner[T]) cloneUint(src reflect.Value) reflect.Value {
	dst := reflect.New(src.Type()).Elem()
	dst.SetUint(src.Uint())

	return dst
}

func (cloner *Cloner[T]) cloneFloat(src reflect.Value) reflect.Value {
	dst := reflect.New(src.Type()).Elem()
	dst.SetFloat(src.Float())

	return dst
}

func (cloner *Cloner[T]) cloneBool(src reflect.Value) reflect.Value {
	dst := reflect.New(src.Type()).Elem()
	dst.SetBool(src.Bool())

	return dst
}

func (cloner *Cloner[T]) cloneString(src reflect.Value) reflect.Value {
	if src, ok := src.Interface().(string); ok {
		return reflect.ValueOf(strings.Clone(src))
	}

	return src
}

func (cloner *Cloner[T]) cloneSlice(src reflect.Value) reflect.Value {
	size := src.Len()
	dst := reflect.MakeSlice(src.Type(), size, size)

	for i := 0; i < size; i++ {
		if val := cloner.cloneValue(src.Index(i)); val.IsValid() {
			dst.Index(i).Set(val)
		}
	}

	return dst
}

func (cloner *Cloner[T]) cloneArray(src reflect.Value) reflect.Value {
	size := src.Type().Len()
	dst := reflect.New(reflect.ArrayOf(size, src.Type().Elem())).Elem()

	for i := 0; i < size; i++ {
		if val := cloner.cloneValue(src.Index(i)); val.IsValid() {
			dst.Index(i).Set(val)
		}
	}

	return dst
}

func (cloner *Cloner[T]) cloneMap(src reflect.Value) reflect.Value {
	dst := reflect.MakeMapWithSize(src.Type(), src.Len())
	iter := src.MapRange()

	for iter.Next() {
		item := cloner.cloneValue(iter.Value())
		key := cloner.cloneValue(iter.Key())
		dst.SetMapIndex(key, item)
	}

	return dst
}

func (cloner *Cloner[T]) clonePointer(src reflect.Value) reflect.Value {
	dst := reflect.New(src.Type().Elem())

	if !src.IsNil() {
		if val := cloner.cloneValue(src.Elem()); val.IsValid() {
			dst.Elem().Set(val)
		}
	}

	return dst
}

func (cloner *Cloner[T]) cloneStruct(src reflect.Value) reflect.Value {
	t := src.Type()
	dst := reflect.New(t)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		var val reflect.Value

		switch field.Tag.Get(fieldTagName) {
		case fieldTagValueSkip, fieldTagValueSkipAlias:
			val = reflect.New(src.Field(i).Type()).Elem()
		case fieldTagValueShadowCopy:
			val = src.Field(i)
		default:
			val = cloner.cloneValue(src.Field(i))
		}

		if val.IsValid() {
			dst.Elem().Field(i).Set(val)
		}
	}

	return dst.Elem()
}
