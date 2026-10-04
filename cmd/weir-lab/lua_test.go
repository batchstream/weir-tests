package main

import (
	"context"
	"strings"
	"testing"
)

func TestLuaComparisonRejectsInvalidWorkloadsBeforeStartup(t *testing.T) {
	cases := []struct {
		name    string
		options labOptions
		message string
	}{
		{name: "client_bulk", options: labOptions{mode: "bulk-saturation", luaMutations: true, writePercent: 100}, message: "single-record"},
		{name: "no_mutations", options: labOptions{mode: "fixed", luaMutations: true}, message: "positive write percentage"},
		{name: "oversized_lua_source", options: labOptions{mode: "fixed", luaMutations: true, writePercent: 100, payload: 256 << 10}, message: "256 KiB"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := runLab(context.Background(), test.options)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatal("invalid comparison reached binary lookup or fixture startup", err)
			}
		})
	}
}
