package output

import (
	"html/template"
	"log"
	"os"
	"path"

	"github.com/pkg/errors"
	"github.com/promlabs/promql-compliance-tester/comparer"
	"github.com/promlabs/promql-compliance-tester/config"
)

var funcMap = map[string]interface{}{
	"include": func(includePassing bool, result *comparer.Result) bool {
		return includePassing || !result.Success()
	},
	"numResults": func(results []*comparer.Result) int {
		return len(results)
	},
	"numPassed": func(results []*comparer.Result) int {
		num := 0
		for _, r := range results {
			if r.Success() {
				num++
			}
		}
		return num
	},
	"numFailed": func(results []*comparer.Result) int {
		num := 0
		for _, r := range results {
			if !r.Success() {
				num++
			}
		}
		return num
	},
	"percent": func(part, total int) float64 {
		return 100 * float64(part) / float64(total)
	},
}

// HTML produces HTML output for a number of query results.
func HTML(tplFile string) (Outputter, error) {
	t, err := template.New(path.Base(tplFile)).Funcs(funcMap).ParseFiles(tplFile)
	if err != nil {
		return nil, errors.Wrapf(err, "parsing template file %q", tplFile)
	}

	return func(results []*comparer.Result, includePassing bool, tweaks []*config.QueryTweak) {
		err := t.Execute(os.Stdout, struct {
			Results        []*comparer.Result
			includePassing bool
		}{
			Results:        results,
			includePassing: includePassing,
		})
		if err != nil {
			log.Println("executing template:", err)
		}
	}, nil
}
