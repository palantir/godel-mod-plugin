// Copyright (c) 2026 Palantir Technologies Inc. All rights reserved.
// Use of this source code is governed by the Apache License, Version 2.0
// that can be found in the LICENSE file.

package gomod

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModVendorGoFlagsSet(t *testing.T) {
	for _, tc := range []struct {
		name   string
		goFlag string
		want   bool
	}{
		{name: "unset"},
		{name: "only vendor mode", goFlag: "-mod=vendor", want: true},
		{name: "among other flags", goFlag: "-x -mod=vendor -count=1", want: true},
		{name: "surrounded by extra whitespace", goFlag: "  -mod=vendor\t-x ", want: true},
		{name: "another mode", goFlag: "-mod=mod"},
		{name: "readonly mode", goFlag: "-mod=readonly"},
		{name: "unrelated flag", goFlag: "-x"},
		{name: "value is not matched as a prefix", goFlag: "-mod=vendoring"},
		{name: "flag is not matched as a suffix", goFlag: "-no-mod=vendor"},
		{name: "value must be attached to the flag", goFlag: "-mod vendor"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GOFLAGS", tc.goFlag)
			assert.Equal(t, tc.want, modVendorGoFlagsSet())
		})
	}
}
