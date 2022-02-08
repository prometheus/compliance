package cases

// AllCases contains all the usable test cases in this package.
// It is recommended to keep the name of rule group same as the corresponding function calls
// for easy debugging.
var AllCases = []TestCase{
	PendingAndFiringAndResolved(),
	PendingAndResolved_AlwaysInactive(),
	ZeroFor_SmallFor(),
	NewAlerts_OrderCheck(),
}
