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
	m := make(template.FuncMap)
	err := tstruct.AddFuncMap[S](m)
	if err != nil {
		t.Fatal(err)
	}
	m["yield"] = func(x any) error {
		want := S{
			URL:  "x",
			Data: map[string]int{"a": 1, "b": 2},
			List: []int{-1, -2},
			ZStr: "zhello",
			Sub:  T{A: "A"},
		}
		if !reflect.DeepEqual(x, want) {
			t.Fatalf("got %#v, want %#v", x, want)
		}
		return nil
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
	p, err := template.New("test").Funcs(m).Parse(tmpl)
	if err != nil {
		t.Fatal(err)
	}
	err = p.Execute(io.Discard, nil)
	if err != nil {
		t.Fatal(err)
	}
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
