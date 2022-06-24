// Package tstruct provides template FuncMap helpers to construct struct literals from a Go template.
//
// See also https://pkg.go.dev/rsc.io/tmplfunc.
package tstruct

import (
	"fmt"
	"reflect"
)

// AddFuncMap adds constructors for T to base.
// base must not be nil.
// AddFuncMap will return an error and leave base unmodified
// if any FuncMap keys will be overwritten or duplicated.
func AddFuncMap[T any](base map[string]any) error {
	if base == nil {
		return fmt.Errorf("base FuncMap is nil")
	}
	var t T
	rv := reflect.ValueOf(t)
	rt := rv.Type()
	fm := make(map[string]any)
	err := addStructMethods(rt, fm)
	if err != nil {
		return err
	}
	// Check all additions before we touch base.
	for k := range fm {
		if _, ok := base[k]; ok {
			return fmt.Errorf("base FuncMap conflict on key %q", k)
		}
	}
	// OK to add.
	for k, v := range fm {
		base[k] = v
	}
	return nil
}

func addStructMethods(rt reflect.Type, fm map[string]any) error {
	// TODO: Accept namespacing prefix(es)?

	// Make a struct constructor with the same name as the struct.
	// It takes as arguments functions that can be applied to modify the struct.
	// We generate functions that return such arguments below.
	if err := hasName(fm, rt.Name()); err != nil {
		return err
	}
	fm[rt.Name()] = func(args ...applyFn) reflect.Value {
		v := reflect.New(rt).Elem()
		for _, apply := range args {
			apply(v)
		}
		return v
	}

	// For each struct field, generate a function that modifies that struct field,
	// named after the struct field.
	// Make args with the same name as each of the struct fields.
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if !f.IsExported() {
			continue
		}

		if f.Type.Kind() == reflect.Interface {
			return fmt.Errorf("interface field %s is not supported", f.Name)
		}
		method, ok := reflect.PtrTo(f.Type).MethodByName("TStructSet")
		if ok {
			if err := hasName(fm, f.Name); err != nil {
				return err
			}
			if method.Type.NumOut() != 0 {
				return fmt.Errorf("(*%v).TStructSet (for field %s) must not return values", f.Type.Name(), f.Name)
			}
			if _, ok := f.Type.MethodByName("TStructSet"); ok {
				return fmt.Errorf("(%v).TStructSet (for field %s) must have pointer receiver", f.Type.Name(), f.Name)
			}
			fm[f.Name] = func(args ...reflect.Value) applyFn {
				return func(v reflect.Value) {
					x := reflect.New(method.Type.In(0).Elem())
					dvArgs := make([]reflect.Value, len(args))
					for i, arg := range args {
						dvArgs[i] = devirt(arg)
					}
					args = append([]reflect.Value{x}, dvArgs...)
					method.Func.Call(args)
					v.FieldByIndex(f.Index).Set(x.Elem())
				}
			}
			continue
		}

		if f.Type.Kind() == reflect.Struct {
			// Process this struct as well.
			// TODO: avoid panic on recursively defined structs (but really, don't do that)
			err := addStructMethods(f.Type, fm)
			if err != nil {
				return err
			}
		}

		name := f.Name
		var fn any
		switch f.Type.Kind() {
		case reflect.Map:
			// TODO: name = "Set" + name?
			fn = func(k, e reflect.Value) applyFn {
				return func(dst reflect.Value) {
					f := dst.FieldByIndex(f.Index)
					if f.IsZero() {
						f.Set(reflect.MakeMap(f.Type()))
					}
					f.SetMapIndex(devirt(k), devirt(e))
				}
			}
		case reflect.Slice:
			// TODO: name = "Append" + name?
			fn = func(e reflect.Value) applyFn {
				return func(dst reflect.Value) {
					f := dst.FieldByIndex(f.Index)
					f.Set(reflect.Append(f, devirt(e)))
				}
			}
		// TODO: reflect.Array: Set by index with a func named AtName? Does it even matter?
		default:
			// TODO: name = "Set" + name?
			fn = func(x reflect.Value) applyFn {
				return func(dst reflect.Value) {
					dst.FieldByIndex(f.Index).Set(devirt(x))
				}
			}
		}
		if err := hasName(fm, name); err != nil {
			return err
		}
		fm[name] = fn
	}
	return nil
}

func hasName(fm map[string]any, name string) error {
	_, ok := fm[name]
	if ok {
		return fmt.Errorf("duplicate FuncMap name %s", name)
	}
	return nil
}

type applyFn = func(v reflect.Value)

// devirt makes x have a concrete type.
// TODO: is there a better/cheaper way to do this?
func devirt(x reflect.Value) reflect.Value {
	if x.Type().Kind() == reflect.Interface {
		x = reflect.ValueOf(x.Interface())
	}
	return x
}
