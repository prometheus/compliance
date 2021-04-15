package output

import (
	"fmt"
	"github.com/promlabs/promql-compliance-tester/comparer"
	"github.com/promlabs/promql-compliance-tester/config"
)

// TSV produces tab separated values output for a number of query results.
func TSV(results []*comparer.Result, passing bool, tweaks []*config.QueryTweak) {
	successes := 0
	unsupported := 0

	fmt.Println("QUERY\tSTART\tSTOP\tSTEP\tRESULT")

	for _, res := range results {
		if res.Success() {
			successes++
		}
		if res.Unsupported {
			unsupported++
		}

		fmt.Printf("%v\t%v\t%v\t%v\t", res.TestCase.Query, res.TestCase.Start, res.TestCase.End, res.TestCase.Resolution)
		if res.Success() {
			fmt.Println("PASSED")
		} else if res.Unsupported {
			fmt.Println("UNSUPPORTED")
		} else {
			fmt.Println("FAILED")
		}
	}
	totalTestCases := len(results)
	totalFailed := totalTestCases - successes - unsupported
	fmt.Printf("\n\t\tPASSED\t%v\t%.4f\n", successes, float64(successes)/float64(totalTestCases))
	fmt.Printf("\t\tFAILED\t%v\t%.4f\n", totalFailed, float64(totalFailed)/float64(totalTestCases))
	fmt.Printf("\t\tUNSUPPORTED\t%v\t%.4f\n", unsupported, float64(unsupported)/float64(totalTestCases))
	fmt.Printf("\t\tTOTAL\t%v\t%.4f\n", totalTestCases, float64(1))
}
