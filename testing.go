// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package compliance

import (
	"os"
	"runtime/debug"
	"testing"
)

// RFCLevel represents the compliance requirement level of a test case.
type RFCLevel string

const (
	// MustLevel marks requirements mandated by the specification.
	MustLevel RFCLevel = "MUST"
	// ShouldLevel marks requirements recommended by the specification.
	ShouldLevel RFCLevel = "SHOULD"
	// MayLevel marks optional capabilities defined by the specification.
	MayLevel RFCLevel = "MAY"
	// RecommendedLevel marks behaviours outside the specification that are recommended for Prometheus compatibility or performance.
	RecommendedLevel RFCLevel = "RECOMMENDED"
)

// Annotate records the RFC requirement level on t as the "rfcLevel" test attribute.
func (r RFCLevel) Annotate(t *testing.T) {
	t.Attr("rfcLevel", string(r))
}

// AnnotateTarget records the tested target name on t as the "rw" test attribute consumed by common/index.html.
func AnnotateTarget(t *testing.T, target string) {
	t.Attr("rw", target)
}

// AnnotateDescription records a non-empty test description on t as the "description" test attribute.
func AnnotateDescription(t *testing.T, desc string) {
	if desc == "" {
		return
	}
	t.Attr("description", desc)
}

// SafeRun executes f inside a subtest named name, annotating target, rfc, and desc test attributes.
// Unless DEBUG is non-empty, panics inside f are recovered and reported via t.Fatalf so a single
// panicking case does not abort the rest of the compliance suite.
func SafeRun(t *testing.T, name, target, desc string, rfc RFCLevel, f func(t *testing.T)) bool {
	t.Helper()
	return t.Run(name, func(t *testing.T) {
		AnnotateTarget(t, target)
		rfc.Annotate(t)
		AnnotateDescription(t, desc)

		if os.Getenv("DEBUG") == "" {
			// Capture panics only in non-debug runs so debugging preserves the raw panic trace.
			defer func() {
				if val := recover(); val != nil {
					t.Fatalf("test panicked: %v\n%s", val, debug.Stack())
				}
			}()
		}
		f(t)
	})
}

// ValidateFunc validates a captured test result R.
type ValidateFunc[R any] func(t *testing.T, res R)

// ValidateCase defines a validation subtest with its own RFC requirement level and description.
// Sharing one target setup across multiple ValidateCase entries allows checking MUST, SHOULD,
// and RECOMMENDED assertions without re-running target setup per assertion.
type ValidateCase[R any] struct {
	// Name is a unique subtest name under the parent test.
	Name string
	// Description describes what this validation subtest checks.
	Description string
	// RFCLevel is the compliance requirement level for this validation subtest.
	RFCLevel RFCLevel
	// Validate checks res and fails t when the requirement is violated.
	Validate ValidateFunc[R]
}

// RunValidateCases executes each validation subtest in cases via SafeRun.
// Subtests still run and fail immediately when setupOk is false so every case appears in the result matrix.
func RunValidateCases[R any](t *testing.T, target string, setupOk bool, res R, cases []ValidateCase[R]) {
	t.Helper()
	for _, vc := range cases {
		SafeRun(t, vc.Name, target, vc.Description, vc.RFCLevel, func(t *testing.T) {
			if !setupOk {
				t.Fatal("setup failed")
			}
			vc.Validate(t, res)
		})
	}
}
