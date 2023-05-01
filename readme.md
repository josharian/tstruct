Package tstruct provides template FuncMap helpers to construct struct literals within a Go template.

In your Go code:

```go
type T struct {
    S string
    N int
    M map[string]int
    L []float64
}

m := template.FuncMap{ /* your func map here */ }
err := tstruct.AddFuncMap[T](m)
// handle err
```

This will register FuncMap functions called `T`, `S`, `N`, `M`, and `L`, for the struct name and each of its fields. You can use them to construct and populate a T from a template:

```
{{ template "template-that-renders-T" T
  (S "a string")
  (L 1.0 2.0)
  (M "one map entry" 1)
  (M "another map entry" 2)
  (M "x" 3 "y" 4)
  (N 42)
  (L 4.0)
}}
```

And voila: Your template will be called with a struct equal to:

```go
T{
    S: "a string",
    N: 42,
    M: map[string]int{"one map entry": 1, "another map entry": 2, "x": 3, "y": 4},
    L: []float64{1.0, 2.0, 4.0},
}
```

Note that order is irrelevant, except for slice appends.

As a special case (matching package flag), you may omit the argument `true` when setting a bool field to true: `(Enabled)` is equivalent to `(Enabled true)`.

If you have multiple struct types whose fields share a name, the field setters will Just Work, despite having a single name. However, no two struct types may share a name, nor can a struct type and a field share a name.

To request that tstruct ignore a struct field, add the struct tag `tstruct:"-"` to it.

If you need to construct an unusual type from a template, there's a magic method: `TStructSet`. To use it, declare a type that has that method on a pointer receiver. It can accept any number of args, which will be passed directly from the template args. In the method, set the value according to the args.

Example:

```go
type Repeat string

func (x *Repeat) TStructSet(s string, count int) {
	*x = Repeat(strings.Repeat(s, count))
}

type U struct {
    S Repeat
}
```

In your template:

```
{{ $lab := U (S "hi " 15) }}
```

That creates a `U`, and populates its `S` field by calling `(*Repeat).TStructSet` with arguments `"hi "` and `15`.

The conflict with the field named `S` in type `T` is handled automatically.

---

If this is not what you wanted, you might check out https://pkg.go.dev/rsc.io/tmplfunc.

If this is _almost_ what you wanted, but not quite, tell me about it. :)
