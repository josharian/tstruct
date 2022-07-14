package tstruct_test

import (
	"io"
	"reflect"
	"testing"
	"text/template"

	"github.com/josharian/tstruct"
)

type Z string

func (z *Z) TStructSet(x string) {
	*z = "z" + Z(x)
}

type S struct {
	URL  string
	Data map[string]int
	List []int
	ZStr Z
	Sub  T
}

type T struct {
	A string
}

func TestBasic(t *testing.T) {
	want := S{
		URL:  "x",
		Data: map[string]int{"a": 1, "b": 2},
		List: []int{-1, -2},
		ZStr: "zhello",
		Sub:  T{A: "A"},
	}
	const tmpl = `
{{ yield
	(S
		(URL "x")
		(Data "a" 1)
		(Data "b" 2)
		(List -1)
		(List -2)
		(ZStr "hello")
		(Sub (T (A "A")))
	)
}}
`
	testOne(t, want, tmpl)
}

func TestDevirtualization(t *testing.T) {
	m := make(template.FuncMap)
	err := tstruct.AddFuncMap[S](m)
	if err != nil {
		t.Fatal(err)
	}
	m["yield"] = func(x any) error {
		want := S{
			URL:  "a",
			Data: map[string]int{"a": 1},
			List: []int{1},
			ZStr: "za",
			Sub:  T{A: "a"},
		}
		if !reflect.DeepEqual(x, want) {
			t.Fatalf("got %#v, want %#v", x, want)
		}
		return nil
	}
	const tmpl = `
{{ yield
	(S
		(URL .Str)
		(Data .Str .Int)
		(List .Int)
		(ZStr .Str)
		(Sub (T (A .Str)))
	)
}}
`
	p, err := template.New("test").Funcs(m).Parse(tmpl)
	if err != nil {
		t.Fatal(err)
	}
	err = p.Execute(io.Discard, map[string]any{"Str": "a", "Int": 1})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFieldReuse(t *testing.T) {
	type X struct {
		F int
	}
	type Y struct {
		F string
	}
	type W struct {
		F Z
	}
	m := make(template.FuncMap)
	err := tstruct.AddFuncMap[X](m)
	if err != nil {
		t.Fatal(err)
	}
	err = tstruct.AddFuncMap[Y](m)
	if err != nil {
		t.Fatal(err)
	}
	err = tstruct.AddFuncMap[W](m)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	m["yield"] = func(x any) error {
		calls++
		switch x.(type) {
		case X:
			want := X{F: 1}
			if !reflect.DeepEqual(x, want) {
				t.Fatalf("got %#v, want %#v", x, want)
			}
		case Y:
			want := Y{F: "a"}
			if !reflect.DeepEqual(x, want) {
				t.Fatalf("got %#v, want %#v", x, want)
			}
		case W:
			want := W{F: "za"}
			if !reflect.DeepEqual(x, want) {
				t.Fatalf("got %#v, want %#v", x, want)
			}
		default:
			t.Fatalf("unexpected type %T", x)
		}
		return nil
	}
	const tmpl = `{{ yield (X (F 1)) }} {{ yield (Y (F "a")) }} {{ yield (W (F "a")) }}`
	p, err := template.New("test").Funcs(m).Parse(tmpl)
	if err != nil {
		t.Fatal(err)
	}
	err = p.Execute(io.Discard, nil)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatalf("got %d calls, want 3", calls)
	}
}

func TestCollisionDetection(t *testing.T) {
	m := make(template.FuncMap)
	m["S"] = func(x any) error { return nil }
	err := tstruct.AddFuncMap[S](m)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSliceOfStructs(t *testing.T) {
	type Sub struct {
		X int
	}
	type T struct {
		X []Sub
	}
	const tmpl = `{{ yield (T (X (Sub (X 1))) (X (Sub (X 2)))) }}`
	want := T{X: []Sub{{X: 1}, {X: 2}}}
	testOne(t, want, tmpl)
}

func TestAnonymousStructField(t *testing.T) {
	type T struct {
		X struct {
			A int
		}
	}
	m := make(template.FuncMap)
	err := tstruct.AddFuncMap[T](m)
	if err == nil {
		t.Fatalf("expected error, got %#v", m)
	}
}

func TestNonStruct(t *testing.T) {
	type T []int
	m := make(template.FuncMap)
	err := tstruct.AddFuncMap[T](m)
	if err == nil {
		t.Fatalf("expected error, got %#v", m)
	}
}

func TestAppendMany(t *testing.T) {
	type T struct {
		X []int
	}
	want := T{X: []int{1, 2, 3, 4}}
	const tmpl = `{{ yield (T (X 1 2 3) (X 4)) }}`
	testOne(t, want, tmpl)
}

func TestMapMany(t *testing.T) {
	type T struct {
		M map[string]int
	}
	want := T{M: map[string]int{"a": 1, "b": 2, "c": 3}}
	const tmpl = `{{ yield (T (M "a" 1 "b" 2) (M "c" 3)) }}`
	testOne(t, want, tmpl)
}

func TestInterfaceField(t *testing.T) {
	type T struct {
		I any
	}
	testOne(t, T{I: 1}, `{{ yield (T (I 1)) }}`)
	testOne(t, T{I: "a"}, `{{ yield (T (I "a")) }}`)
	testOne(t, T{I: T{I: 1.0}}, `{{ yield (T (I (T (I 1.0)))) }}`)
}

func testOne[T any](t *testing.T, want T, tmpl string) {
	t.Helper()
	m := make(template.FuncMap)
	err := tstruct.AddFuncMap[T](m)
	if err != nil {
		t.Fatal(err)
	}
	m["yield"] = func(x any) error {
		if !reflect.DeepEqual(x, want) {
			t.Fatalf("got %#v, want %#v", x, want)
		}
		return nil
	}
	p, err := template.New("test").Funcs(m).Parse(tmpl)
	if err != nil {
		t.Fatal(err)
	}
	err = p.Execute(io.Discard, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRepeatedSliceStruct(t *testing.T) {
	type A struct {
		I int
	}
	type T struct {
		AA []A
	}
	type U struct {
		AA []A
	}
	// Check that it is possible to add T and U to a single FuncMap.
	m := make(template.FuncMap)
	err := tstruct.AddFuncMap[T](m)
	if err != nil {
		t.Fatal(err)
	}
	err = tstruct.AddFuncMap[U](m)
	if err != nil {
		t.Fatal(err)
	}
	// Check that it behaves correctly.
	type V struct {
		ET T
		EU U
	}
	want := V{
		ET: T{AA: []A{{I: 1}, {I: 2}}},
		EU: U{AA: []A{{I: 1}, {I: 2}}},
	}
	const tmpl = `{{ yield
(V
	(ET (T (AA (A (I 1)) (A (I 2)))))
	(EU (U (AA (A (I 1)) (A (I 2)))))
)
}}`
	testOne(t, want, tmpl)
}
