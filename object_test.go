package goja

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/dop251/goja/unistring"
)

func TestDefineProperty(t *testing.T) {
	r := New()
	o := r.NewObject()

	err := o.DefineDataProperty("data", r.ToValue(42), FLAG_TRUE, FLAG_TRUE, FLAG_TRUE)
	if err != nil {
		t.Fatal(err)
	}

	err = o.DefineAccessorProperty("accessor_ro", r.ToValue(func() int {
		return 1
	}), nil, FLAG_TRUE, FLAG_TRUE)
	if err != nil {
		t.Fatal(err)
	}

	err = o.DefineAccessorProperty("accessor_rw",
		r.ToValue(func(call FunctionCall) Value {
			return o.Get("__hidden")
		}),
		r.ToValue(func(call FunctionCall) (ret Value) {
			o.Set("__hidden", call.Argument(0))
			return
		}),
		FLAG_TRUE, FLAG_TRUE)

	if err != nil {
		t.Fatal(err)
	}

	if v := o.Get("accessor_ro"); v.ToInteger() != 1 {
		t.Fatalf("Unexpected accessor value: %v", v)
	}

	err = o.Set("accessor_ro", r.ToValue(2))
	if err == nil {
		t.Fatal("Expected an error")
	}
	if ex, ok := err.(*Exception); ok {
		if msg := ex.Error(); msg != "TypeError: Cannot assign to read only property 'accessor_ro'" {
			t.Fatalf("Unexpected error: '%s'", msg)
		}
	} else {
		t.Fatalf("Unexected error type: %T", err)
	}

	err = o.Set("accessor_rw", 42)
	if err != nil {
		t.Fatal(err)
	}

	if v := o.Get("accessor_rw"); v.ToInteger() != 42 {
		t.Fatalf("Unexpected value: %v", v)
	}
}

func TestPropertyOrder(t *testing.T) {
	const SCRIPT = `
	var o = {};
	var sym1 = Symbol(1);
	var sym2 = Symbol(2);
	o[sym2] = 1;
	o[4294967294] = 1;
	o[2] = 1;
	o[1] = 1;
	o[0] = 1;
	o["02"] = 1;
	o[4294967295] = 1;
	o["01"] = 1;
	o["00"] = 1;
	o[sym1] = 1;
	var expected = ["0", "1", "2", "4294967294", "02", "4294967295", "01", "00", sym2, sym1];
	var actual = Reflect.ownKeys(o);
	if (actual.length !== expected.length) {
		throw new Error("Unexpected length: "+actual.length);
	}
	for (var i = 0; i < actual.length; i++) {
		if (actual[i] !== expected[i]) {
			throw new Error("Unexpected list: " + actual);
		}
	}
	`

	testScript(SCRIPT, _undefined, t)
}

func TestDefinePropertiesSymbol(t *testing.T) {
	const SCRIPT = `
	var desc = {};
	desc[Symbol.toStringTag] = {value: "Test"};
	var o = {};
	Object.defineProperties(o, desc);
	o[Symbol.toStringTag] === "Test";
	`

	testScript(SCRIPT, valueTrue, t)
}

func TestObjectShorthandProperties(t *testing.T) {
	const SCRIPT = `
	var b = 1;
	var a = {b, get() {return "c"}};

	assert.sameValue(a.b, b, "#1");
	assert.sameValue(a.get(), "c", "#2");

	var obj = {
		w\u0069th() { return 42; }
    };

	assert.sameValue(obj['with'](), 42, 'property exists');
	`
	testScriptWithTestLib(SCRIPT, _undefined, t)
}

func TestObjectAssign(t *testing.T) {
	const SCRIPT = `
	assert.sameValue(Object.assign({ b: 1 }, { get a() {
          Object.defineProperty(this, "b", {
            value: 3,
            enumerable: false
          });
        }, b: 2 }).b, 1, "#1");

	assert.sameValue(Object.assign({ b: 1 }, { get a() {
          delete this.b;
        }, b: 2 }).b, 1, "#2");
	`
	testScriptWithTestLib(SCRIPT, _undefined, t)
}

func TestExportCircular(t *testing.T) {
	vm := New()
	o := vm.NewObject()
	o.Set("o", o)
	v := o.Export()
	if m, ok := v.(map[string]interface{}); ok {
		if reflect.ValueOf(m["o"]).Pointer() != reflect.ValueOf(v).Pointer() {
			t.Fatal("Unexpected value")
		}
	} else {
		t.Fatal("Unexpected type")
	}

	res, err := vm.RunString(`var a = []; a[0] = a;`)
	if err != nil {
		t.Fatal(err)
	}
	v = res.Export()
	if a, ok := v.([]interface{}); ok {
		if reflect.ValueOf(a[0]).Pointer() != reflect.ValueOf(v).Pointer() {
			t.Fatal("Unexpected value")
		}
	} else {
		t.Fatal("Unexpected type")
	}
}

type test_s struct {
	S *test_s1
}
type test_s1 struct {
	S *test_s
}

func TestExportToCircular(t *testing.T) {
	vm := New()
	o := vm.NewObject()
	o.Set("o", o)
	var m map[string]interface{}
	err := vm.ExportTo(o, &m)
	if err != nil {
		t.Fatal(err)
	}

	type K string
	type T map[K]T
	var m1 T
	err = vm.ExportTo(o, &m1)
	if err != nil {
		t.Fatal(err)
	}

	type A []A
	var a A
	res, err := vm.RunString("var a = []; a[0] = a;")
	if err != nil {
		t.Fatal(err)
	}
	err = vm.ExportTo(res, &a)
	if err != nil {
		t.Fatal(err)
	}
	if &a[0] != &a[0][0] {
		t.Fatal("values do not match")
	}

	o = vm.NewObject()
	o.Set("S", o)
	var s test_s
	err = vm.ExportTo(o, &s)
	if err != nil {
		t.Fatal(err)
	}
	if s.S.S != &s {
		t.Fatalf("values do not match: %v, %v", s.S.S, &s)
	}

	type test_s2 struct {
		S  interface{}
		S1 *test_s2
	}

	var s2 test_s2
	o.Set("S1", o)

	err = vm.ExportTo(o, &s2)
	if err != nil {
		t.Fatal(err)
	}

	if m, ok := s2.S.(map[string]interface{}); ok {
		if reflect.ValueOf(m["S"]).Pointer() != reflect.ValueOf(m).Pointer() {
			t.Fatal("Unexpected m.S")
		}
	} else {
		t.Fatalf("Unexpected s2.S type: %T", s2.S)
	}
	if s2.S1 != &s2 {
		t.Fatal("Unexpected s2.S1")
	}

	o1 := vm.NewObject()
	o1.Set("S", o)
	o1.Set("S1", o)
	err = vm.ExportTo(o1, &s2)
	if err != nil {
		t.Fatal(err)
	}
	if s2.S1.S1 != s2.S1 {
		t.Fatal("Unexpected s2.S1.S1")
	}
}

func TestExportWrappedMap(t *testing.T) {
	vm := New()
	m := map[string]interface{}{
		"test": "failed",
	}
	exported := vm.ToValue(m).Export()
	if exportedMap, ok := exported.(map[string]interface{}); ok {
		exportedMap["test"] = "passed"
		if v := m["test"]; v != "passed" {
			t.Fatalf("Unexpected m[\"test\"]: %v", v)
		}
	} else {
		t.Fatalf("Unexpected export type: %T", exported)
	}
}

func TestExportToWrappedMap(t *testing.T) {
	vm := New()
	m := map[string]interface{}{
		"test": "failed",
	}
	var exported map[string]interface{}
	err := vm.ExportTo(vm.ToValue(m), &exported)
	if err != nil {
		t.Fatal(err)
	}
	exported["test"] = "passed"
	if v := m["test"]; v != "passed" {
		t.Fatalf("Unexpected m[\"test\"]: %v", v)
	}
}

func TestExportToWrappedMapCustom(t *testing.T) {
	type CustomMap map[string]bool
	vm := New()
	m := CustomMap{}
	var exported CustomMap
	err := vm.ExportTo(vm.ToValue(m), &exported)
	if err != nil {
		t.Fatal(err)
	}
	exported["test"] = true
	if v := m["test"]; v != true {
		t.Fatalf("Unexpected m[\"test\"]: %v", v)
	}
}

func TestExportToSliceNonIterable(t *testing.T) {
	vm := New()
	o := vm.NewObject()
	var a []interface{}
	err := vm.ExportTo(o, &a)
	if err == nil {
		t.Fatal("Expected an error")
	}
	if len(a) != 0 {
		t.Fatalf("a: %v", a)
	}
	if msg := err.Error(); msg != "cannot convert [object Object] to []interface {}: not an array or iterable" {
		t.Fatalf("Unexpected error: %v", err)
	}
}

func ExampleRuntime_ExportTo_iterableToSlice() {
	vm := New()
	v, err := vm.RunString(`
	function reverseIterator() {
	    const arr = this;
	    let idx = arr.length;
	    return {
			next: () => idx > 0 ? {value: arr[--idx]} : {done: true}
	    }
	}
	const arr = [1,2,3];
	arr[Symbol.iterator] = reverseIterator;
	arr;
	`)
	if err != nil {
		panic(err)
	}

	var arr []int
	err = vm.ExportTo(v, &arr)
	if err != nil {
		panic(err)
	}

	fmt.Println(arr)
	// Output: [3 2 1]
}

func TestRuntime_ExportTo_proxiedIterableToSlice(t *testing.T) {
	vm := New()
	v, err := vm.RunString(`
	function reverseIterator() {
	    const arr = this;
	    let idx = arr.length;
	    return {
			next: () => idx > 0 ? {value: arr[--idx]} : {done: true}
	    }
	}
	const arr = [1,2,3];
	arr[Symbol.iterator] = reverseIterator;
	new Proxy(arr, {});
	`)
	if err != nil {
		t.Fatal(err)
	}

	var arr []int
	err = vm.ExportTo(v, &arr)
	if err != nil {
		t.Fatal(err)
	}
	if out := fmt.Sprint(arr); out != "[3 2 1]" {
		t.Fatal(out)
	}
}

func ExampleRuntime_ExportTo_arrayLikeToSlice() {
	vm := New()
	v, err := vm.RunString(`
	({
		length: 3,
		0: 1,
		1: 2,
		2: 3
	});
	`)
	if err != nil {
		panic(err)
	}

	var arr []int
	err = vm.ExportTo(v, &arr)
	if err != nil {
		panic(err)
	}

	fmt.Println(arr)
	// Output: [1 2 3]
}

func TestExportArrayToArrayMismatchedLengths(t *testing.T) {
	vm := New()
	a := vm.NewArray(1, 2)
	var a1 [3]int
	err := vm.ExportTo(a, &a1)
	if err == nil {
		t.Fatal("expected error")
	}
	if msg := err.Error(); !strings.Contains(msg, "lengths mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExportIterableToArrayMismatchedLengths(t *testing.T) {
	vm := New()
	a, err := vm.RunString(`
		new Map([[1, true], [2, true]]);
	`)
	if err != nil {
		t.Fatal(err)
	}

	var a1 [3]interface{}
	err = vm.ExportTo(a, &a1)
	if err == nil {
		t.Fatal("expected error")
	}
	if msg := err.Error(); !strings.Contains(msg, "lengths mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExportArrayLikeToArrayMismatchedLengths(t *testing.T) {
	vm := New()
	a, err := vm.RunString(`
		({
			length: 2,
			0: true,
			1: true
		});
	`)
	if err != nil {
		t.Fatal(err)
	}

	var a1 [3]interface{}
	err = vm.ExportTo(a, &a1)
	if err == nil {
		t.Fatal("expected error")
	}
	if msg := err.Error(); !strings.Contains(msg, "lengths mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSetForeignReturnValue(t *testing.T) {
	const SCRIPT = `
	var array = [1, 2, 3];
	var arrayTarget = new Proxy(array, {});

	Object.preventExtensions(array);

	!Reflect.set(arrayTarget, "foo", 2);
	`

	testScript(SCRIPT, valueTrue, t)
}

func TestDefinePropertiesUndefinedVal(t *testing.T) {
	const SCRIPT = `
var target = {};
var sym = Symbol();
target[sym] = 1;
target.foo = 2;
target[0] = 3;

var getOwnKeys = [];
var proxy = new Proxy(target, {
  getOwnPropertyDescriptor: function(_target, key) {
    getOwnKeys.push(key);
  },
});

Object.defineProperties({}, proxy);
	true;
	`

	testScript(SCRIPT, valueTrue, t)
}

func ExampleObject_Delete() {
	vm := New()
	obj := vm.NewObject()
	_ = obj.Set("test", true)
	before := obj.Get("test")
	_ = obj.Delete("test")
	after := obj.Get("test")
	fmt.Printf("before: %v, after: %v", before, after)
	// Output: before: true, after: <nil>
}

func BenchmarkPut(b *testing.B) {
	v := &Object{}

	o := &baseObject{
		val:        v,
		extensible: true,
	}
	v.self = o

	o.init()

	var key Value = asciiString("test")
	var val Value = valueInt(123)

	for i := 0; i < b.N; i++ {
		v.setOwn(key, val, false)
	}
}

func BenchmarkPutStr(b *testing.B) {
	v := &Object{}

	o := &baseObject{
		val:        v,
		extensible: true,
	}

	o.init()

	v.self = o

	var val Value = valueInt(123)

	for i := 0; i < b.N; i++ {
		o.setOwnStr("test", val, false)
	}
}

func BenchmarkGet(b *testing.B) {
	v := &Object{}

	o := &baseObject{
		val:        v,
		extensible: true,
	}

	o.init()

	v.self = o
	var n Value = asciiString("test")

	for i := 0; i < b.N; i++ {
		v.get(n, nil)
	}

}

func BenchmarkGetStr(b *testing.B) {
	v := &Object{}

	o := &baseObject{
		val:        v,
		extensible: true,
	}
	v.self = o

	o.init()

	for i := 0; i < b.N; i++ {
		o.getStr("test", nil)
	}
}

func __toString(v Value) string {
	switch v := v.(type) {
	case asciiString:
		return string(v)
	default:
		return ""
	}
}

func BenchmarkToString1(b *testing.B) {
	v := asciiString("test")

	for i := 0; i < b.N; i++ {
		v.toString()
	}
}

func BenchmarkToString2(b *testing.B) {
	v := asciiString("test")

	for i := 0; i < b.N; i++ {
		__toString(v)
	}
}

func BenchmarkConv(b *testing.B) {
	count := int64(0)
	for i := 0; i < b.N; i++ {
		count += valueInt(123).ToInteger()
	}
	if count == 0 {
		b.Fatal("zero")
	}
}

func BenchmarkToUTF8String(b *testing.B) {
	var s valueString = asciiString("test")
	for i := 0; i < b.N; i++ {
		_ = s.String()
	}
}

func BenchmarkAdd(b *testing.B) {
	var x, y Value
	x = valueInt(2)
	y = valueInt(2)

	for i := 0; i < b.N; i++ {
		if xi, ok := x.(valueInt); ok {
			if yi, ok := y.(valueInt); ok {
				x = xi + yi
			}
		}
	}
}

func BenchmarkAddString(b *testing.B) {
	var x, y Value

	tst := asciiString("22")
	x = asciiString("2")
	y = asciiString("2")

	for i := 0; i < b.N; i++ {
		var z Value
		if xi, ok := x.(valueString); ok {
			if yi, ok := y.(valueString); ok {
				z = xi.concat(yi)
			}
		}
		if !z.StrictEquals(tst) {
			b.Fatalf("Unexpected result %v", x)
		}
	}
}

func TestBaseObjectMemUsage(t *testing.T) {
	tests := []struct {
		name           string
		val            *baseObject
		threshold      int
		expectedMem    uint64
		expectedNewMem uint64
		errExpected    error
	}{
		{
			name:           "should have a value of SizeEmptyStruct given a nil object",
			threshold:      100,
			val:            nil,
			expectedMem:    SizeEmptyStruct,
			expectedNewMem: SizeEmptyStruct,
			errExpected:    nil,
		},
		{
			name:           "should have a value of SizeEmptyStruct given an empty object",
			threshold:      100,
			val:            &baseObject{},
			expectedMem:    SizeEmptyStruct,
			expectedNewMem: SizeEmptyStruct,
			errExpected:    nil,
		},
		{
			name:      "should account for each key value pair given a non-empty object",
			threshold: 100,
			val:       &baseObject{propNames: []unistring.String{"test"}, values: map[unistring.String]Value{"test": valueInt(99)}},
			// overhead + len("test") with string overhead + value
			expectedMem: SizeEmptyStruct + (4 + SizeString) + SizeInt,
			// overhead + len("test") with string overhead + value
			expectedNewMem: SizeEmptyStruct + (4 + SizeString) + SizeInt,
			errExpected:    nil,
		},
		{
			name:      "should account for each key value pair given a non-empty object with a nil value",
			threshold: 100,
			val:       &baseObject{propNames: []unistring.String{"test"}, values: map[unistring.String]Value{"test": nil}},
			// overhead + len("test") with string overhead
			expectedMem: SizeEmptyStruct + (4 + SizeString),
			// overhead + len("test") with string overhead
			expectedNewMem: SizeEmptyStruct + (4 + SizeString),
			errExpected:    nil,
		},
		{
			name:      "should account for sampled key value pair given a non-empty object over threshold",
			threshold: 20,
			val: &baseObject{
				propNames: []unistring.String{
					"test0",
					"test1",
					"test2",
					"test3",
				},
				values: map[unistring.String]Value{
					"test0": valueInt(99),
					"test1": valueInt(99),
					"test2": valueInt(99),
					"test3": valueInt(99),
				},
			},
			// overhead + len("testN") with string overhead + value
			expectedMem: SizeEmptyStruct + ((5+SizeString)+SizeInt)*4,
			// overhead + len("testN") with string overhead + value
			expectedNewMem: SizeEmptyStruct + ((5+SizeString)+SizeInt)*4,
			errExpected:    nil,
		},
		{
			name:      "should account for prototype's given an object with a valid prototype",
			threshold: 100,
			val:       &baseObject{prototype: &Object{}},
			// baseObject overhead + prototype overhead
			expectedMem: SizeEmptyStruct + SizeEmptyStruct,
			// baseObject overhead + prototype overhead
			expectedNewMem: SizeEmptyStruct + SizeEmptyStruct,
			errExpected:    nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			total, newTotal, err := tc.val.MemUsage(NewMemUsageContext(New(), 100, 100, 100, tc.threshold, nil))
			if err != tc.errExpected {
				t.Fatalf("Unexpected error. Actual: %v Expected: %v", err, tc.errExpected)
			}
			if err != nil && tc.errExpected != nil && err.Error() != tc.errExpected.Error() {
				t.Fatalf("Errors do not match. Actual: %v Expected: %v", err, tc.errExpected)
			}
			if total != tc.expectedMem {
				t.Fatalf("Unexpected memory return. Actual: %v Expected: %v", total, tc.expectedMem)
			}
			if newTotal != tc.expectedNewMem {
				t.Fatalf("Unexpected new memory return. Actual: %v Expected: %v", newTotal, tc.expectedNewMem)
			}
		})
	}
}

func TestPrimitiveValueObjectMemUsage(t *testing.T) {
	tests := []struct {
		name           string
		val            *primitiveValueObject
		expectedMem    uint64
		expectedNewMem uint64
		errExpected    error
	}{
		{
			name:           "should have a value of SizeEmptyStruct given a nil primitive value object",
			val:            nil,
			expectedMem:    SizeEmptyStruct,
			expectedNewMem: SizeEmptyStruct,
			errExpected:    nil,
		},
		{
			name:           "should have a value of SizeEmptyStruct given an empty primitive value object",
			val:            &primitiveValueObject{},
			expectedMem:    SizeEmptyStruct,
			expectedNewMem: SizeEmptyStruct,
			errExpected:    nil,
		},
		{
			name: "should account for overhead given a primitive value object with empty object",
			val:  &primitiveValueObject{baseObject: baseObject{}},
			// baseObject overhead + len("test") + value
			expectedMem: SizeEmptyStruct,
			// baseObject overhead + len("test") with string overhead + value
			expectedNewMem: SizeEmptyStruct,
			errExpected:    nil,
		},
		{
			name: "should account for overehead and each key value pair given a primitive value object with non-empty object",
			val:  &primitiveValueObject{baseObject: baseObject{propNames: []unistring.String{"test"}, values: map[unistring.String]Value{"test": valueInt(99)}}},
			// baseObject overhead + len("test") with string overhead + value
			expectedMem: SizeEmptyStruct + (4 + SizeString) + SizeInt,
			// baseObject overhead + len("test") with string overhead + value
			expectedNewMem: SizeEmptyStruct + (4 + SizeString) + SizeInt,
			errExpected:    nil,
		},
		{
			name: "should account for pValue given a primitive value object with non-empty pValue",
			val:  &primitiveValueObject{pValue: valueInt(99)},
			// baseObject overhead + value
			expectedMem: SizeEmptyStruct + SizeInt,
			// baseObject overhead + value
			expectedNewMem: SizeEmptyStruct + SizeInt,
			errExpected:    nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			total, newTotal, err := tc.val.MemUsage(NewMemUsageContext(New(), 100, 100, 100, 100, nil))
			if err != tc.errExpected {
				t.Fatalf("Unexpected error. Actual: %v Expected: %v", err, tc.errExpected)
			}
			if err != nil && tc.errExpected != nil && err.Error() != tc.errExpected.Error() {
				t.Fatalf("Errors do not match. Actual: %v Expected: %v", err, tc.errExpected)
			}
			if total != tc.expectedMem {
				t.Fatalf("Unexpected memory return. Actual: %v Expected: %v", total, tc.expectedMem)
			}
			if newTotal != tc.expectedNewMem {
				t.Fatalf("Unexpected new memory return. Actual: %v Expected: %v", newTotal, tc.expectedNewMem)
			}
		})
	}
}
