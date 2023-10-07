// Package tstruct provides template FuncMap helpers to construct struct literals within a Go template.
//
// See also https://pkg.go.dev/rsc.io/tmplfunc.
//
// TODO: Unify docs with README, link to blog post (if I ever write one).
package tstruct

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// AddFuncMap adds constructors for T to base.
// base must not be nil.
// AddFuncMap will return an error if there is a conflict with any existing entries in base.
// AddFuncMap may modify entries in base that were added by a prior call to AddFuncMap.
// If AddFuncMap returns a non-nil error, base will be unmodified.
func AddFuncMap[T any](base map[string]any) error {
	if base == nil {
		return fmt.Errorf("base FuncMap is nil")
	}
	var t T
	rt := reflect.TypeOf(t)
	origrt := rt
	if rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	if rt.Kind() != reflect.Struct {
		return fmt.Errorf("non-struct type %v", rt)
	}
	// Make a copy of base to modify.
	// This is safe because all keys are strings and all values are funcs.
	fnmap := make(map[string]any)
	copyFuncMap(fnmap, base)
	// Add struct and field funcs to fnmap.
	err := addStructFuncs[T](origrt, fnmap)
	if err != nil {
		return err
	}
	// Nothing went wrong; copy our modified FuncMap back onto base.
	copyFuncMap(base, fnmap)
	return nil
}

func copyFuncMap(dst, src map[string]any) {
	for k, v := range src {
		dst[k] = v
	}
}

// addStructFuncs adds funcs to fnmap to construct structs of type rt and to populate rt's fields.
func addStructFuncs[T any](rt reflect.Type, fnmap map[string]any) error {
	origrt := rt
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}
	if rt.Name() == "" {
		return fmt.Errorf("anonymous struct (type %v) is not supported", rt)
	}
	// TODO: Accept namespacing prefix(es)?

	// Make a struct constructor for rt with the same name as the struct.
	// It takes as arguments functions that can be applied to modify the struct.
	// We generate functions that return such arguments below.
	if x, ok := fnmap[rt.Name()]; ok {
		match := registeredFuncMatches[T](x, rt)
		if !match {
			return fmt.Errorf("conflicting FuncMap entries for %s: %T", rt.Name(), x)
		}
		// We already have a constructor for this struct type.
		// Replace it with a more precisely typed one, if possible.
		// But if T is reflect.Value, we risk overwriting a more precisely typed function.
		var zero T
		if reflect.TypeOf(zero) == reflectValueType {
			return nil
		}
	}

	var required fieldsAreUnset
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if !f.IsExported() || f.Tag.Get("tstruct") != "+" {
			continue
		}
		// Require that this struct field be set.
		if required == nil {
			required = make(fieldsAreUnset)
		}
		required[f.Name] = true
	}

	fnmap[rt.Name()] = func(args ...applyFn) T {
		v := reflect.New(rt).Elem()
		// If there are required fields, check whether they are about to be set.
		if required != nil {
			// clone required
			// TODO: when Go 1.21 is out, use maps.Clone
			r2 := make(fieldsAreUnset, len(required))
			for k, v := range required {
				r2[k] = v
			}
			rqv := reflect.ValueOf(r2)
			// Call apply using our special sentinel map type.
			// Each apply function will delete the field name it is responsible for
			// from the map, but not do any further work.
			for _, apply := range args {
				apply(rqv)
			}
			// Gather all unset required fields.
			if len(r2) > 0 {
				missing := make([]string, 0, len(r2))
				for k := range r2 {
					missing = append(missing, rt.Name()+"."+k)
				}
				sort.Strings(missing)
				panic(fmt.Sprintf("%s required but not provided", strings.Join(missing, ", ")))
			}
		}
		// Now, actually set the fields.
		for _, apply := range args {
			apply(v)
		}
		var t T
		switch any(t).(type) {
		case reflect.Value:
			// We want to return a reflect.Value.
			// v already is a reflect.Value.
			// (We know that; help the compiler.)
			return any(v).(T)
		}
		if origrt.Kind() == reflect.Pointer {
			v = v.Addr()
		}
		// v holds a T. Extract it.
		return v.Interface().(T)
	}

	// For each struct field, generate a function that modifies that struct field,
	// named after the struct field.
	// Make args with the same name as each of the struct fields.
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if !f.IsExported() {
			continue
		}
		if f.Tag.Get("tstruct") == "-" {
			// Ignore this struct field.
			continue
		}
		switch f.Type.Kind() {
		case reflect.Struct:
			// Process this struct's fields as well!
			// TODO: avoid panic on recursively defined structs (but really, don't do that)
			err := addStructFuncs[reflect.Value](f.Type, fnmap)
			if err != nil {
				return err
			}
		case reflect.Slice:
			if elem := f.Type.Elem(); elem.Kind() == reflect.Struct {
				err := addStructFuncs[reflect.Value](elem, fnmap)
				if err != nil {
					return err
				}
			}
		case reflect.Map:
			for _, elem := range []reflect.Type{f.Type.Key(), f.Type.Elem()} {
				if elem.Kind() == reflect.Struct {
					err := addStructFuncs[reflect.Value](elem, fnmap)
					if err != nil {
						return err
					}
				}
			}
		}
		name := f.Name
		// TODO: modify fn name based on field type? E.g. AppendF for a field named F of slice type?
		fn, err := genSavedApplyFnForField(f, name)
		if err != nil {
			return err
		}
		err = setSavedApplyFn(fnmap, name, rt, fn)
		if err != nil {
			return err
		}
	}
	return nil
}

func registeredFuncMatches[T any](x any, rt reflect.Type) bool {
	// There's already a registered function with the name we want to use.
	// If it is a tstruct constructor for the exact same type as we are
	// trying to generate now, that's ok. Otherwise, fail.
	_, isTypedCtor := x.(func(args ...applyFn) T)
	if isTypedCtor {
		// OK
		return true
	}
	// Check whether x is a func(args ...applyFn) T for any T, including possibly reflect.Value.
	// If so, call x to get a struct value whose type we can inspect.
	xfn := reflect.ValueOf(x)
	if xfn.Kind() != reflect.Func {
		return false
	}
	xType := xfn.Type()
	if xType.NumIn() != 1 || xType.NumOut() != 1 || !xType.IsVariadic() {
		return false
	}
	in := xType.In(0)
	if in.Kind() != reflect.Slice || in.Elem() != applyFnType {
		return false
	}
	out := xfn.Call(nil)
	s := out[0]
	// If s is a reflect.Value (holding a reflect.Value!), use its contents instead.
	if s.Type() == reflectValueType {
		s = s.Interface().(reflect.Value)
	}
	return s.Type() == rt
}

// fieldsAreUnset is a special sentinel type that applyFn recognizes.
// It is a map from a field name to whether it remains unset.
type fieldsAreUnset map[string]bool

var fieldsAreUnsetType = reflect.TypeOf(fieldsAreUnset(nil))

// didMarkFieldAsSet checks whether this is a request to mark the field name as having been set by an apply function.
// If it returns true, the apply function must stop processing v.
func didMarkFieldAsSet(v reflect.Value, name string) bool {
	if v.Type() != fieldsAreUnsetType {
		return false
	}
	// Update the map: This field is no longer unset.
	m := v.Interface().(fieldsAreUnset)
	delete(m, name)
	return true
}

// genSavedApplyFnForField generates a savedApplyFn for f, to be given name name.
func genSavedApplyFnForField(f reflect.StructField, name string) (savedApplyFn, error) {
	method, ok := reflect.PtrTo(f.Type).MethodByName("TStructSet")
	if ok {
		if method.Type.NumOut() != 0 {
			return nil, fmt.Errorf("(*%v).TStructSet (for field %s) must not return values", f.Type.Name(), f.Name)
		}
		if _, ok := f.Type.MethodByName("TStructSet"); ok {
			return nil, fmt.Errorf("(%v).TStructSet (for field %s) must have pointer receiver", f.Type.Name(), f.Name)
		}
		return func(args ...reflect.Value) applyFn {
			return func(v reflect.Value) {
				if didMarkFieldAsSet(v, name) {
					return
				}
				x := reflect.New(method.Type.In(0).Elem())
				dvArgs := devirtAll(args)
				args = append([]reflect.Value{x}, dvArgs...)
				method.Func.Call(args)
				convertAndSet(v.FieldByIndex(f.Index), x.Elem())
			}
		}, nil
	}

	switch f.Type.Kind() {
	case reflect.Map:
		return func(args ...reflect.Value) applyFn {
			return func(dst reflect.Value) {
				if didMarkFieldAsSet(dst, name) {
					return
				}
				f := dst.FieldByIndex(f.Index)
				if f.IsZero() {
					f.Set(reflect.MakeMap(f.Type()))
				}
				if len(args) == 1 {
					// If it is a map arg with appropriate types, copy the elems over.
					arg := devirt(args[0])
					typ := arg.Type()
					ftyp := f.Type()
					if typ.Kind() == reflect.Map && typ.Key().AssignableTo(ftyp.Key()) && typ.Elem().AssignableTo(ftyp.Elem()) {
						iter := arg.MapRange()
						for iter.Next() {
							f.SetMapIndex(iter.Key(), iter.Value())
						}
						// success
						return
					}
				}
				if len(args)&1 != 0 {
					panic(fmt.Sprintf("odd number of args to %v, expected (key, elem) pairs, got %d args", name, len(args)))
				}
				for i := 0; i < len(args); i += 2 {
					k := args[i]
					e := args[i+1]
					f.SetMapIndex(devirt(k), devirt(e))
				}
			}
		}, nil
	case reflect.Slice:
		return func(args ...reflect.Value) applyFn {
			return func(dst reflect.Value) {
				if didMarkFieldAsSet(dst, name) {
					return
				}
				f := dst.FieldByIndex(f.Index)
				for _, arg := range devirtAll(args) {
					if arg.Type().AssignableTo(f.Type()) {
						f.Set(reflect.AppendSlice(f, arg))
					} else {
						f.Set(reflect.Append(f, arg))
					}
				}
			}
		}, nil
		// TODO: reflect.Array: Set by index with a func named AtName? Does it even matter?
	}
	// Everything else: do a plain Set
	return func(args ...reflect.Value) applyFn {
		return func(dst reflect.Value) {
			if didMarkFieldAsSet(dst, name) {
				return
			}
			out := dst.FieldByIndex(f.Index)
			var x reflect.Value
			switch len(args) {
			case 0:
				// special case for ergonomics: treat (X) as (X true) when destination has bool type
				if out.Type().Kind() == reflect.Bool {
					x = reflect.ValueOf(true)
				}
			case 1:
				x = args[0]
			}
			if !x.IsValid() {
				panic("wrong number of args to " + name + ", expected 1")
			}
			convertAndSet(out, devirt(x))
		}
	}, nil
}

func setSavedApplyFn(fnmap map[string]any, name string, typ reflect.Type, fn savedApplyFn) error {
	existing, ok := fnmap[name]
	if !ok {
		// We are the first ones to use this function name.
		fnmap[name] = fn
		return nil
	}
	dispatch, ok := existing.(savedApplyFn)
	if !ok {
		// Someone has used this name for something other than a savedApplyFn.
		// Refuse to overwrite it.
		return fmt.Errorf("conflicting FuncMap entries for %s", name)
	}
	// We previously used this name for a savedApplyFn.
	// This happens when two structs share the same field name.
	// In that case, replace the function with a new function
	// that checks whether we're being applied to the right struct type,
	// and if not, dispatches to the previous savedApplyFn.
	fnmap[name] = func(args ...reflect.Value) applyFn {
		return func(dst reflect.Value) {
			if didMarkFieldAsSet(dst, name) {
				return
			}
			if dst.Type() == typ {
				// We can handle this type! Do it.
				fn(args...)(dst)
				return
			}
			// Dispatch to a previous function, in the hopes
			// that it can handle this unknown type.
			dispatch(args...)(dst)
		}
	}
	return nil
}

// A savedApplyFn accepts arguments from a template and saves them to be applied later.
type savedApplyFn = func(args ...reflect.Value) applyFn

// An applyFn applies previously saved arguments to v.
type applyFn = func(v reflect.Value)

// TODO: use reflect.TypeFor once Go 1.22 comes out
var (
	applyFnType      = reflect.TypeOf(applyFn(nil))
	reflectValueType = reflect.TypeOf(reflect.Value{})
)

// devirt makes x have a concrete type.
func devirt(x reflect.Value) reflect.Value {
	if x.Type().Kind() == reflect.Interface {
		x = x.Elem()
	}
	return x
}

// devirtAll returns a copy of s containing devirtualized values.
func devirtAll(s []reflect.Value) []reflect.Value {
	c := make([]reflect.Value, len(s))
	for i, x := range s {
		c[i] = devirt(x)
	}
	return c
}

func convertAndSet(dst, src reflect.Value) {
	dst.Set(src.Convert(dst.Type()))
}
