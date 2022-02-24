package cases

import "sort"

// AllCasesMap contains all the usable test cases in this package.
// It is recommended to keep the name of rule group same as the corresponding function calls
// for easy debugging.
var AllCasesMap = map[string]TestCase{
	"PendingAndFiringAndResolved":       PendingAndFiringAndResolved(),
	"PendingAndResolved_AlwaysInactive": PendingAndResolved_AlwaysInactive(),
	"ZeroFor_SmallFor":                  ZeroFor_SmallFor(),
	"NewAlerts_OrderCheck":              NewAlerts_OrderCheck(),
}

func AllCases() []TestCase {
	allCases := make([]TestCase, 0, len(AllCasesMap))
	for _, c := range AllCasesMap {
		allCases = append(allCases, c)
	}
	sort.Slice(allCases, func(i, j int) bool {
		gi, _ := allCases[i].Describe()
		gj, _ := allCases[j].Describe()
		return gi < gj
	})
	return allCases
}
