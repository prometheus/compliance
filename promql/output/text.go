package output

import (
	"fmt"
	"strings"

	"github.com/promlabs/promql-compliance-tester/comparer"
	"github.com/promlabs/promql-compliance-tester/config"
)

// Text produces text-based output for a number of query results.
func Text(results []*comparer.Result, includePassing bool, tweaks []*config.QueryTweak) {
	successes := 0
	unsupported := 0
	for _, res := range results {
		if res.Success() {
			successes++
			if !includePassing {
				continue
			}
		}
		if res.Unsupported {
			unsupported++
		}

		fmt.Println(strings.Repeat("-", 80))
		fmt.Printf("QUERY: %v\n", res.TestCase.Query)
		fmt.Printf("START: %v, STOP: %v, STEP: %v\n", res.TestCase.Start, res.TestCase.End, res.TestCase.Resolution)
		fmt.Printf("RESULT: ")
		if res.Success() {
			fmt.Println("PASSED")
		} else if res.Unsupported {
			fmt.Println("UNSUPPORTED: ")
			fmt.Printf("Query is unsupported: %v\n", res.UnexpectedFailure)
		} else {
			fmt.Printf("FAILED: ")
			if res.UnexpectedFailure != "" {
				fmt.Printf("Query failed unexpectedly: %v\n", res.UnexpectedFailure)
			}
			if res.UnexpectedSuccess {
				fmt.Println("Query succeeded, but should have failed.")
			}
			if res.Diff != "" {
				fmt.Println("Query returned different results:")
				fmt.Println(res.Diff)
			}
		}
	}

	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("General query tweaks:")
	if len(tweaks) == 0 {
		fmt.Println("None.")
	}
	for _, t := range tweaks {
		fmt.Println("* ", t.Note)
	}
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Total: %d / %d (%.2f%%) passed, %d unsupported\n", successes, len(results), 100*float64(successes)/float64(len(results)), unsupported)
}
