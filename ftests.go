package main

// https://tales.mbivert.com/on-a-function-based-test-framework/

import (
	"testing"
	"reflect"
	"strings"
	"runtime"
	"fmt"
	"encoding/json" // pretty-printing
)

type test struct {
	name     string
	fun      interface{}
	args     []interface{}
	expected []interface{}
}

func getFn(f interface{}) string {
	xs := strings.Split((runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()), ".")
	return xs[len(xs)-1]
}

func doTest(t *testing.T, f interface{}, args []interface{}, expected []interface{}) {
	// []interface{} -> []reflect.Value
	var vargs []reflect.Value
	for _, v := range args {
		vargs = append(vargs, reflect.ValueOf(v))
	}

	got := reflect.ValueOf(f).Call(vargs)

	// []reflect.Value -> []interface{}
	var igot []interface{}
	for _, v := range got {
		igot = append(igot, v.Interface())
	}

	if !reflect.DeepEqual(igot, expected) {
		sgot, err := json.MarshalIndent(igot, "", "\t")
		if err != nil {
			sgot = []byte(fmt.Sprintf("%+v (%s)", igot, err))
		}
		sexp, err := json.MarshalIndent(expected, "", "\t")
		if err != nil {
			sexp = []byte(fmt.Sprintf("%+v (%s)", expected, err))
		}
		// >= 4 and we get nothing; 3 is asm, 2 is testing, 1 is doTests()
		// not sure we can do better
/*
		_, fn, l, ok := runtime.Caller(3)
		if !ok {
			fn = "???"
			l = 0
		}
		fmt.Printf("%s:%d got: '%s', expected: '%s'", fn, l, igot, expected)
*/
		// meh, error are printed as {} with JSON.
		fmt.Printf("got: '%s', expected: '%s'", igot, expected)
		t.Fatalf("got: '%s', expected: '%s'", sgot, sexp)
	}
}

func doTests(t *testing.T, tests []test) {
	for _, test := range tests {
		t.Run(fmt.Sprintf("%s()/%s", getFn(test.fun), test.name), func(t *testing.T) {
			doTest(t, test.fun, test.args, test.expected)
		})
	}
}
