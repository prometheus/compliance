package testsuite

import (
	"io"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/compliance/alert_generator/cases"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/notifier"
)

type AlertMessageParser func(b []byte) ([]notifier.Alert, error)

type alertsServer struct {
	logger log.Logger

	server         *http.Server
	serverErr      error
	serverCloseErr error
	closeC         chan struct{}

	expectedAlertsMtx sync.Mutex
	expectedAlerts    map[string]*expectedAlerts

	messageParser AlertMessageParser

	errsMtx sync.Mutex
	errs    map[string]*allErrs

	wg sync.WaitGroup

	disabled bool
}

type expectedAlerts struct {
	alerts []cases.ExpectedAlert
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
	t             time.Time
	expectedAlert cases.ExpectedAlert
	alert         notifier.Alert
	err           error
}

type unexpectedErr struct {
	t     time.Time
	alert notifier.Alert
}

// TODO: assumes resend delay of 1m.
func newAlertsServer(port string, disabled bool, logger log.Logger, messageParser AlertMessageParser) *alertsServer {
	as := &alertsServer{
		logger:         log.With(logger, "component", "alertsServer"),
		errs:           make(map[string]*allErrs),
		expectedAlerts: make(map[string]*expectedAlerts),
		closeC:         make(chan struct{}),
		disabled:       disabled,
		messageParser:  messageParser,
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
	// Some proxies/sinks like webhook.site might use a preflight check.
	if req.Method == http.MethodOptions {
		res.Header().Set("Access-Control-Allow-Origin", "*")
		res.Header().Set("Access-Control-Allow-Method", "POST")
		res.WriteHeader(http.StatusOK)
		return
	}
	now := time.Now().UTC()
	b, err := io.ReadAll(req.Body)
	if err != nil {
		level.Error(as.logger).Log("msg", "Error in reading request body", "err", err.Error())
		res.WriteHeader(http.StatusBadRequest) // Or is it 500?
		return
	}

	alerts, err := as.messageParser(b)
	if err != nil {
		level.Error(as.logger).Log("msg", "Error in parsing request body", "err", err.Error())
		res.WriteHeader(http.StatusBadRequest)
		return
	}

	level.Info(as.logger).Log("msg", "Received alerts", "num_alerts", len(alerts))
	as.expectedAlertsMtx.Lock()

	var addBack []cases.ExpectedAlert
	var missedAlerts []cases.ExpectedAlert

	// Alerts that matched. This will be used to adjust the time for the next resend.
	success := make(map[string]cases.ExpectedAlert)
	for _, al := range alerts {
		id := al.Labels.String()
		exp := as.getPossibleAlert(now, id)
		errs := as.getErr(al.Labels.Get("rulegroup"))
		if len(exp) == 0 {
			errs.unexpectedAlerts = append(errs.unexpectedAlerts, unexpectedErr{
				t:     now,
				alert: al,
			})
			continue
		}

		var me *matchingErr
		var idx int
		for i, ex := range exp {
			err := ex.Matches(now, al)
			if err == nil {
				// We found a match.
				success[id] = ex
				idx = i
				me = nil
				break
			}
			if me == nil {
				// We only report the first matching error.
				me = &matchingErr{
					t:             now,
					expectedAlert: ex,
					alert:         al,
					err:           err,
				}
			}
		}

		if me == nil {
			// We are expecting these alert to come later.
			addBack = append(addBack, exp[idx+1:]...)
			// These are missed, were expected before.
			lastResendWasIgnored := false
			for _, ma := range exp[:idx] {
				if !ma.CanBeIgnored() {
					if lastResendWasIgnored && ma.Resend {
						// If the last resend was ignored, it means this resend can
						// also be ignored since this alert's time would not be updated
						// yet and can give false positive as missed alert.
						continue
					}
					lastResendWasIgnored = false
					missedAlerts = append(missedAlerts, ma)
				} else {
					lastResendWasIgnored = ma.Resend
				}
			}

		} else {
			// None matches. Put back the alerts to match future alerts.
			addBack = append(addBack, exp...)
			errs.matchingErrs = append(errs.matchingErrs, *me)
		}
	}
	as.addExpectedAlerts(addBack...)
	as.addMissedAlerts(missedAlerts)

	// Since the alert is sent with a "resend delay" w.r.t. the last time the alert was sent,
	// the delay might drift upto 1 group interval everytime. So for the Nth resend, the interval
	// between the first alert sent and Nth resend can be upto N*(ResendDelay+GroupInterval), and not N*ResendDelay.
	// Hence, we adjust our time expectation for the next alert if it is supposed to be a resend.
Outer2:
	for id, sa := range success {
		eas := as.expectedAlerts[id]
		if len(eas.alerts) == 0 {
			continue
		}
		for i := range eas.alerts {
			if labels.Compare(eas.alerts[i].Alert.Labels, sa.Alert.Labels) != 0 {
				continue
			}
			if !eas.alerts[i].Resend {
				continue Outer2
			}
			eas.alerts[i].Ts = now.Add(cases.ResendDelay - cases.MaxRTT)
		}
	}

	as.expectedAlertsMtx.Unlock()

	res.Header().Set("Access-Control-Allow-Origin", "*")
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
	seen := make(map[string]struct{})
	for _, a := range alerts {
		id := a.Alert.Labels.String()
		ea := as.expectedAlerts[id]
		if ea == nil {
			ea = &expectedAlerts{}
			as.expectedAlerts[id] = ea
		}
		ea.alerts = append(ea.alerts, a)
		seen[id] = struct{}{}
	}
	for id := range seen {
		ea := as.expectedAlerts[id]
		sort.Slice(ea.alerts, func(i, j int) bool {
			return ea.alerts[i].OrderingID < ea.alerts[j].OrderingID
		})
	}
}

// getPossibleAlert gives possible alerts for the given time and labels and removes
// old alerts from the list.
func (as *alertsServer) getPossibleAlert(now time.Time, lblsString string) []cases.ExpectedAlert {
	var alerts []cases.ExpectedAlert

	// The additional allocations for every call is a design choice to keep the code simple
	// since the absolute size of total allocations will be tiny.
	var missedAlerts []cases.ExpectedAlert

	for id, eas := range as.expectedAlerts {
		var newExpAlerts []cases.ExpectedAlert
		for _, ea := range eas.alerts {
			if ea.ShouldBeIgnored() {
				continue
			}
			// For the first alert that comes, the remote write RTT can add to some delay. Hence 2*RTT.
			tolerance := ea.TimeTolerance + (2 * cases.MaxRTT)
			if ea.Ts.Add(tolerance).Before(now) {
				if !ea.CanBeIgnored() {
					missedAlerts = append(missedAlerts, ea)
				}
			} else if id == lblsString && now.After(ea.Ts) && now.Before(ea.Ts.Add(tolerance)) {
				alerts = append(alerts, ea)
			} else {
				newExpAlerts = append(newExpAlerts, ea)
			}
		}
		as.expectedAlerts[id].alerts = newExpAlerts
	}

	as.addMissedAlerts(missedAlerts)

	return alerts
}

// missedAlertCleanup cleans up the missed alerts.
func (as *alertsServer) missedAlertCleanup(now time.Time) {
	var missedAlerts []cases.ExpectedAlert

	for id, eas := range as.expectedAlerts {
		var newExpAlerts []cases.ExpectedAlert
		for _, ea := range eas.alerts {
			if ea.ShouldBeIgnored() {
				continue
			}
			// For the first alert that comes, the remote write RTT can add to some delay. Hence 2*RTT.
			if ea.Ts.Add(ea.TimeTolerance + (2 * cases.MaxRTT)).Before(now) {
				if !ea.CanBeIgnored() {
					missedAlerts = append(missedAlerts, ea)
				}
			} else {
				newExpAlerts = append(newExpAlerts, ea)
			}
		}
		as.expectedAlerts[id].alerts = newExpAlerts
	}

	as.addMissedAlerts(missedAlerts)
}

func (as *alertsServer) addMissedAlerts(missedAlerts []cases.ExpectedAlert) {
	for _, sa := range missedAlerts {
		errs := as.getErr(sa.Alert.Labels.Get("rulegroup"))
		errs.missedAlerts = append(errs.missedAlerts, sa)
	}
}

func (as *alertsServer) Start() {
	if as.disabled {
		return
	}
	as.wg.Add(2)
	go func() {
		defer as.wg.Done()
		as.serverErr = as.server.ListenAndServe()
	}()
	go func() {
		defer as.wg.Done()
		for {
			select {
			case <-as.closeC:
				return
			case <-time.After(1 * time.Minute):
				as.missedAlertCleanup(time.Now().UTC())
			}
		}
	}()
}

func (as *alertsServer) Stop() {
	if as.disabled {
		return
	}
	close(as.closeC)
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
	return NewMulti(
		errors.Wrap(as.serverErr, "http server"),
		errors.Wrap(as.serverCloseErr, "http server close"),
	).Err()
}

func (as *alertsServer) groupError() map[string]*allErrs {
	if as.disabled {
		return map[string]*allErrs{}
	}
	return as.errs
}

func (as *alertsServer) groupsFacingErrors() map[string]bool {
	if as.disabled {
		return map[string]bool{}
	}
	as.errsMtx.Lock()
	defer as.errsMtx.Unlock()

	g := make(map[string]bool, len(as.errs))
	for rg, err := range as.errs {
		if len(err.missedAlerts)+len(err.unexpectedAlerts)+len(err.matchingErrs) > 0 {
			g[rg] = true
		}
	}

	return g
}

func (as *alertsServer) expectedAlertsError() map[string][]cases.ExpectedAlert {
	if as.disabled {
		return map[string][]cases.ExpectedAlert{}
	}
	as.expectedAlertsMtx.Lock()
	defer as.expectedAlertsMtx.Unlock()

	eae := make(map[string][]cases.ExpectedAlert)
	for rg, ea := range as.expectedAlerts {
		for _, al := range ea.alerts {
			if !al.Resend {
				eae[rg] = append(eae[rg], al)
			}
		}
	}

	return eae
}
