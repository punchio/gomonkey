package gomonkey

import (
	"fmt"
	"reflect"
	"syscall"
	"unsafe"
)

type Patches struct {
	originals map[reflect.Value][]byte
}

type Values []interface{}
type Output struct {
	Values Values
	Times  int
}

func ApplyFunc(target, double interface{}) *Patches {
	return New().ApplyFunc(target, double)
}

func ApplyMethod(target reflect.Type, methodName string, double interface{}) *Patches {
	return New().ApplyMethod(target, methodName, double)
}

func ApplyFuncSeq(target interface{}, doubles []Output) *Patches {
	return New().ApplyFuncSeq(target, doubles)
}

func ApplyMethodSeq(target reflect.Type, methodName string, doubles []Output) *Patches {
	return New().ApplyMethodSeq(target, methodName, doubles)
}

func New() *Patches {
	return &Patches{make(map[reflect.Value][]byte)}
}

func (this *Patches) ApplyFunc(target, double interface{}) *Patches {
	t := reflect.ValueOf(target)
	d := reflect.ValueOf(double)
	return this.applyCore(t, d)
}

func (this *Patches) ApplyMethod(target reflect.Type, methodName string, double interface{}) *Patches {
	m, ok := target.MethodByName(methodName)
	if !ok {
		panic("retrieve method by name failed")
	}
	d := reflect.ValueOf(double)
	return this.applyCore(m.Func, d)
}

func (this *Patches) ApplyFuncSeq(target interface{}, doubles []Output) *Patches {
	funcType := reflect.TypeOf(target)
	if funcType.NumOut() != len(doubles[0].Values) {
		panic(fmt.Sprintf("func type has %v return values, but only %v values provided as double",
			funcType.NumOut(), len(doubles[0].Values)))
	}

	slice := make([]Values, 0)
	for _, double := range doubles {
		t := 0
		if double.Times <= 1 {
			t = 1
		} else {
			t = double.Times
		}
		for j := 0; j < t; j++ {
			slice = append(slice, double.Values)
		}
	}

	t := reflect.ValueOf(target)

	i := 0
	len := len(slice)
	d := reflect.MakeFunc(funcType, func(_ []reflect.Value) []reflect.Value {
		if i < len {
			i++
			return getResultValues(funcType, slice[i-1]...)
		}
		panic("double seq is less than call seq")
	})

	return this.applyCore(t, d)
}

func (this *Patches) ApplyMethodSeq(target reflect.Type, methodName string, doubles []Output) *Patches {
	// TODO
	return this
}

func (this *Patches) Reset() {
	for target, bytes := range this.originals {
		modifyBinary(*(*uintptr)(getPointer(target)), bytes)
		delete(this.originals, target)
	}
}

func (this *Patches) applyCore(target, double reflect.Value) *Patches {
	this.check(target, double)
	original := replace(*(*uintptr)(getPointer(target)), uintptr(getPointer(double)))
	this.originals[target] = original
	return this
}

func (this *Patches) check(target, double reflect.Value) {
	if target.Kind() != reflect.Func {
		panic("target is not a func")
	}

	if double.Kind() != reflect.Func {
		panic("double is not a func")
	}

	if target.Type() != double.Type() {
		panic(fmt.Sprintf("target type(%s) and double type(%s) are different", target.Type(), double.Type()))
	}

	if _, ok := this.originals[target]; ok {
		panic("patch has been existed")
	}
}

func replace(target, double uintptr) []byte {
	code := buildJmpDirective(double)
	bytes := entryAddress(target, len(code))
	original := make([]byte, len(bytes))
	copy(original, bytes)
	modifyBinary(target, code)
	return original
}

func getResultValues(funcType reflect.Type, results ...interface{}) []reflect.Value {
	var resultValues []reflect.Value
	for i, r := range results {
		var resultValue reflect.Value
		if r == nil {
			resultValue = reflect.Zero(funcType.Out(i))
		} else {
			v := reflect.New(funcType.Out(i))
			v.Elem().Set(reflect.ValueOf(r))
			resultValue = v.Elem()
		}
		resultValues = append(resultValues, resultValue)
	}
	return resultValues
}

type value struct {
	_ uintptr
	p unsafe.Pointer
}

func getPointer(v reflect.Value) unsafe.Pointer {
	return (*value)(unsafe.Pointer(&v)).p
}

func entryAddress(p uintptr, l int) []byte {
	return *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{Data: p, Len: l, Cap: l}))
}

func modifyBinary(target uintptr, bytes []byte) {
	function := entryAddress(target, len(bytes))

	page := entryAddress(pageStart(target), syscall.Getpagesize())
	err := syscall.Mprotect(page, syscall.PROT_READ|syscall.PROT_WRITE|syscall.PROT_EXEC)
	if err != nil {
		panic(err)
	}
	copy(function, bytes)

	err = syscall.Mprotect(page, syscall.PROT_READ|syscall.PROT_EXEC)
	if err != nil {
		panic(err)
	}
}

func pageStart(ptr uintptr) uintptr {
	return ptr & ^(uintptr(syscall.Getpagesize() - 1))
}
