package testsuite

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/notifier"
	"github.com/prometheus/prometheus/pkg/labels"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"

	"github.com/prometheus/compliance/alert_generator/cases"
)

type alertsServer struct {
	logger log.Logger

	server         *http.Server
	serverErr      error
	serverCloseErr error

	expectedAlertsMtx sync.Mutex
	expectedAlerts    []cases.ExpectedAlert

	errsMtx sync.Mutex
	errs    map[string]*allErrs

	wg sync.WaitGroup
}

type allErrs struct {
	// Alerts that were received in the expected range.
	missedAlerts []cases.ExpectedAlert
	// Alerts that were received unexpectedly, either being different alerts
	// or alerts outside the expected range or duplicate.
	unexpectedAlerts []unexpectedErr

	matchingErrs []matchingErr
}

type matchingErr struct {
	t     time.Time
	alert notifier.Alert
	err   error
}

type unexpectedErr struct {
	t     time.Time
	alert notifier.Alert
}

// TODO notes:
// ts is the start of when the alert is expected.
// The StartsAt comes from the test cases
// If this alert is resolved, the EndsAt is when it was resolved. It comes from test cases as well.
// EndsAt is now+4m (with tolerance) assuming 1m of resend delay.

// TODO: assumes resend delay of 1m.
func newAlertsServer(port string, logger log.Logger) *alertsServer {
	as := &alertsServer{
		logger: log.WithPrefix(logger, "component", "alertsServer"),
	}
	as.server = &http.Server{
		Addr:         ":" + port, // TODO: take this as a config.
		Handler:      as,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	return as
}

func (as *alertsServer) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	now := time.Now().UTC()
	b, err := ioutil.ReadAll(req.Body)
	if err != nil {
		level.Error(as.logger).Log("msg", "Error in reading request body", "err", err.Error())
		res.WriteHeader(http.StatusBadRequest) // Or is it 500?
		return
	}

	var alerts []notifier.Alert
	err = json.Unmarshal(b, &alerts)
	if err != nil {
		level.Error(as.logger).Log("msg", "Error in unmarshaling request body", "err", err.Error())
		res.WriteHeader(http.StatusBadRequest)
		return
	}

	// TODO: check alerts
	fmt.Println("GOT ALERTS", time.Now().UTC().String(), alerts)
	as.expectedAlertsMtx.Lock()
	var addBack []cases.ExpectedAlert
Outer:
	for _, al := range alerts {
		exp := as.getPossibleAlert(now, al.Labels)
		errs := as.getErr(al.Labels.Get("rulegroup"))
		if len(exp) == 0 {
			errs.unexpectedAlerts = append(errs.unexpectedAlerts, unexpectedErr{
				t:     now,
				alert: al,
			})
			continue
		}

		var me *matchingErr
		for i, ex := range exp {
			err := ex.Matches(now, al)
			if err == nil {
				// We found a match. No need to report error and add back remaining expected alerts.
				if i != len(exp)-1 {
					addBack = append(addBack, exp[i+1:]...)
				}
				continue Outer
			}
			// Add this back into expectedAlerts. This might be some delayed alert and the correct alert
			// might be on its way.
			addBack = append(addBack, ex)
			if me == nil {
				// We only report the first matching error.
				me = &matchingErr{
					t:     now,
					alert: al,
					err:   err,
				}
			}
		}
		errs.matchingErrs = append(errs.matchingErrs, *me)
	}
	as.addExpectedAlerts(addBack...)
	as.expectedAlertsMtx.Unlock()

	res.WriteHeader(http.StatusOK)
}

func (as *alertsServer) getErr(rg string) *allErrs {
	as.errsMtx.Lock()
	defer as.errsMtx.Unlock()

	ae, ok := as.errs[rg]
	if !ok {
		ae = &allErrs{}
		as.errs[rg] = ae
	}
	return ae
}

func (as *alertsServer) addExpectedAlerts(alerts ...cases.ExpectedAlert) {
	as.expectedAlerts = append(as.expectedAlerts, alerts...)
	sort.Slice(as.expectedAlerts, func(i, j int) bool {
		return as.expectedAlerts[i].Ts.Before(as.expectedAlerts[j].Ts)
	})
}

// getPossibleAlert gives possible alerts for the given time and labels and removes
// old alerts from the list.
func (as *alertsServer) getPossibleAlert(now time.Time, lbls labels.Labels) []cases.ExpectedAlert {
	var alerts []cases.ExpectedAlert

	// The additional allocations for every call is a design choice to keep the code simple
	// since the absolute size of total allocations will be tiny.
	staleAlerts := make(map[string][]cases.ExpectedAlert)
	var newExpAlerts []cases.ExpectedAlert

	for _, ea := range as.expectedAlerts {
		rg := ea.Alert.Labels.Get("rulegroup")
		if ea.Ts.Add(ea.TimeTolerance).Before(now) {
			staleAlerts[rg] = append(staleAlerts[rg], ea)
			continue
		}
		if labels.Compare(lbls, ea.Alert.Labels) == 0 && now.After(ea.Ts) && now.Before(ea.Ts.Add(ea.TimeTolerance)) {
			alerts = append(alerts, ea)
			continue
		}
		newExpAlerts = append(newExpAlerts, ea)
	}

	for rg, sa := range staleAlerts {
		errs := as.getErr(rg)
		errs.missedAlerts = append(errs.missedAlerts, sa...)
	}
	as.expectedAlerts = newExpAlerts

	return alerts
}

func (as *alertsServer) Start() {
	as.wg.Add(1)
	go func() {
		defer as.wg.Done()
		as.serverErr = as.server.ListenAndServe()
	}()
}

func (as *alertsServer) Stop() {
	as.serverCloseErr = as.server.Close()
}

func (as *alertsServer) Wait() {
	as.wg.Wait()
}

// TODO: maybe send different errors separately.
// running error, unexpected alerts, missed alerts, errors when matching alerts.
func (as *alertsServer) runningError() error {
	if as.serverErr == http.ErrServerClosed {
		as.serverErr = nil
	}
	return tsdb_errors.NewMulti(
		errors.Wrap(as.serverErr, "http server"),
		errors.Wrap(as.serverCloseErr, "http server close"),
	).Err()
}

func (as *alertsServer) groupError() map[string]*allErrs {
	return as.errs
}

func (as *alertsServer) groupsFacingErrors() map[string]struct{} {
	as.errsMtx.Lock()
	defer as.errsMtx.Unlock()

	g := make(map[string]struct{}, len(as.errs))
	for rg := range as.errs {
		g[rg] = struct{}{}
	}

	return g
}
