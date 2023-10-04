package tstruct_test

import (
	"io"
	"reflect"
	"strings"
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

type WrapS struct {
	Inner S
}

type (
	sFn  = func(...func(reflect.Value)) S
	wsFn = func(...func(reflect.Value)) WrapS
	rvFn = func(...func(reflect.Value)) reflect.Value
)

func TestFieldReuseOuterInner(t *testing.T) {
	m := make(template.FuncMap)
	err := tstruct.AddFuncMap[WrapS](m)
	if err != nil {
		t.Fatal(err)
	}
	err = tstruct.AddFuncMap[S](m)
	if err != nil {
		t.Fatal(err)
	}
	wantType[sFn](t, m["S"])
	wantType[wsFn](t, m["WrapS"])
}

func TestFieldReuseInnerOuter(t *testing.T) {
	m := make(template.FuncMap)
	err := tstruct.AddFuncMap[S](m)
	if err != nil {
		t.Fatal(err)
	}
	err = tstruct.AddFuncMap[WrapS](m)
	if err != nil {
		t.Fatal(err)
	}
	wantType[sFn](t, m["S"])
	wantType[wsFn](t, m["WrapS"])
}

func wantType[T any](t *testing.T, got any) {
	t.Helper()
	z, ok := got.(T)
	if !ok {
		t.Fatalf("expected %T, got %T", z, got)
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

func TestConvert(t *testing.T) {
	type Int int
	type T struct {
		X Int
	}
	want := T{X: 1}
	const tmpl = `{{ yield (T (X 1)) }}`
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

func testOne[T any](t *testing.T, want T, tmpl string, dots ...any) {
	err := testRunOne[T](t, want, tmpl, dots...)
	if err != nil {
		t.Fatal(err)
	}
}

func testOneWantErrStrs[T any](t *testing.T, want T, tmpl string, substrs []string, dots ...any) {
	err := testRunOne[T](t, want, tmpl, dots...)
	if err == nil {
		t.Fatal("expected error")
	}
	for _, substr := range substrs {
		if !strings.Contains(err.Error(), substr) {
			t.Errorf("expected error to contain %q, got %q", substr, err)
		}
	}
}

func testRunOne[T any](t *testing.T, want T, tmpl string, dots ...any) error {
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
	var dot any
	if len(dots) == 1 {
		dot = dots[0]
	}
	return p.Execute(io.Discard, dot)
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

func TestStructFieldNameConflict(t *testing.T) {
	type T struct{}
	type S struct {
		T T
	}
	m := make(template.FuncMap)
	err := tstruct.AddFuncMap[S](m)
	if err == nil {
		t.Fatalf("expected error, got %#v", m)
	}
}

func TestIgnoreStructField(t *testing.T) {
	type T struct{}
	type S struct {
		X int
		T T `tstruct:"-"`
	}
	m := make(template.FuncMap)
	err := tstruct.AddFuncMap[S](m)
	if err != nil {
		t.Fatal(err)
	}
	testOne(t, S{X: 1}, `{{ yield (S (X 1)) }}`)
}

func TestBringASliceToASliceFight(t *testing.T) {
	type T struct {
		X []int
	}
	m := make(template.FuncMap)
	err := tstruct.AddFuncMap[T](m)
	if err != nil {
		t.Fatal(err)
	}
	testOne(t, T{X: []int{1, 2, 3}}, `{{ yield (T (X .Ints)) }}`, map[string]any{"Ints": []int{1, 2, 3}})
	testOne(t, T{X: []int{0, 1, 2, 3}}, `{{ yield (T (X 0 .Ints)) }}`, map[string]any{"Ints": []int{1, 2, 3}})
	testOne(t, T{X: []int{0, 1, 2, 3, 4, 1, 2, 3}}, `{{ yield (T (X 0 .Ints 4 .Ints)) }}`, map[string]any{"Ints": []int{1, 2, 3}})
}

func TestDirectMapsWork(t *testing.T) {
	type T struct {
		X map[string]any
	}
	m := make(template.FuncMap)
	err := tstruct.AddFuncMap[T](m)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"A": 1, "B": 2}
	testOne(t, T{X: want}, `{{ yield (T (X .M)) }}`, map[string]any{"M": want})
	testOne(t, T{X: want}, `{{ yield (T (X .M)) }}`, map[string]any{"M": map[string]int{"A": 1, "B": 2}})
}

func TestBoolTrue(t *testing.T) {
	type T struct {
		X bool
	}
	m := make(template.FuncMap)
	err := tstruct.AddFuncMap[T](m)
	if err != nil {
		t.Fatal(err)
	}
	testOne(t, T{X: false}, `{{ yield (T (X false)) }}`, nil)
	testOne(t, T{X: true}, `{{ yield (T (X true)) }}`, nil)
	testOne(t, T{X: true}, `{{ yield (T (X)) }}`, nil)
}

type R struct {
	NotReq    string
	ReqInt    int            `tstruct:"+"`
	ReqSlice  []int          `tstruct:"+"`
	ReqMap    map[string]int `tstruct:"+"`
	ReqStruct S              `tstruct:"+"`
	ReqZStr   Z              `tstruct:"+"`
}

func TestRequired(t *testing.T) {
	// Check that providing required fields works.
	want := R{
		ReqInt:    1,
		ReqSlice:  []int{1},
		ReqMap:    map[string]int{"a": 1},
		ReqStruct: S{URL: "x"},
		ReqZStr:   "zhello",
	}
	const tmpl = `
{{ yield
	(R
		(ReqInt 1)
		(ReqSlice 1)
		(ReqMap "a" 1)
		(ReqStruct (S (URL "x")))
		(ReqZStr "hello")
	)
}}
`
	testOne(t, want, tmpl)
}

func TestRequiredZeroValueIsOK(t *testing.T) {
	// Check that providing required fields works.
	want := R{
		ReqZStr: "z",
		ReqMap:  map[string]int{},
	}
	const tmpl = `
{{ yield
	(R
		(ReqInt 0)
		(ReqSlice)
		(ReqMap)
		(ReqStruct (S))
		(ReqZStr "")
	)
}}
`
	testOne(t, want, tmpl)
}

func TestRequiredMissing(t *testing.T) {
	// Check that we catch and report all missing required fields.
	want := R{
		ReqInt:    0,
		ReqSlice:  []int{1},
		ReqMap:    map[string]int{"a": 1},
		ReqStruct: S{URL: "x"},
		ReqZStr:   "zhello",
	}
	const tmpl = `
{{ yield
	(R
	)
}}
`
	testOneWantErrStrs(t, want, tmpl, []string{"required", "R.ReqInt", "R.ReqSlice", "R.ReqMap", "R.ReqStruct", "R.ReqZStr"})
}

func TestFieldReuseWithRequire(t *testing.T) {
	// Test that we can reuse a field name if one of the uses is required, without interference.
	type X struct {
		F int
	}
	type Y struct {
		F string `tstruct:"+"`
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

	calls := 0
	m["yield"] = func(x any) error {
		calls++
		switch x.(type) {
		case X:
			want := X{}
			if !reflect.DeepEqual(x, want) {
				t.Fatalf("got %#v, want %#v", x, want)
			}
		default:
			t.Fatalf("unexpected type %T", x)
		}
		return nil
	}
	const tmpl = `{{ yield (X) }} {{ yield (Y) }}`
	p, err := template.New("test").Funcs(m).Parse(tmpl)
	if err != nil {
		t.Fatal(err)
	}
	err = p.Execute(io.Discard, nil)
	// ensure X succeeded
	if calls != 1 {
		t.Fatalf("got %d calls, want 1", calls)
	}
	// ensure Y failed, with the right error
	if err == nil {
		t.Fatal("expected error, got none")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected error to contain %q, got %q", "required", err)
	}
}

func TestRequiredTrackingNoSharedState(t *testing.T) {
	// Test that we don't share state between each evaluation of a struct with required fields.
	//
	// We have a required fields.
	// We will evaluate two templates: one with the field, and one without.
	// If we share state, the second evaluation will succeed,
	// because the first evaluation will have marked the field as present.
	type X struct {
		F int `tstruct:"+"`
	}
	m := make(template.FuncMap)
	err := tstruct.AddFuncMap[X](m)
	if err != nil {
		t.Fatal(err)
	}
	st := template.New("test").Funcs(m)
	// This should succeed: required fields are present.
	t0, err := st.New("first").Parse(`{{ (X (F 1)) }}`)
	if err != nil {
		t.Fatal(err)
	}
	err = t0.Execute(io.Discard, nil)
	if err != nil {
		t.Fatal(err)
	}
	// This should fail: required fields are missing.
	t1, err := st.New("second").Parse(`{{ (X) }}`)
	if err != nil {
		t.Fatal(err)
	}
	err = t1.Execute(io.Discard, nil)
	if err == nil {
		t.Fatal("expected error about missing required field F, got none")
	}
}
