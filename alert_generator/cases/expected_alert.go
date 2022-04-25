package cases

import (
	"fmt"
	"net/url"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/notifier"
)

// ExpectedAlert describes the characteristics of a receiving alert.
// The alert is considered as "may or may not come" (hence no error if not received) in these scenarios:
//   1. (Ts + TimeTolerance) crosses the ResolvedTime time when Resolved is false.
//      Because it can get resolved during the tolerance period.
//   2. (Ts + TimeTolerance) crosses ResolvedTime+15m when Resolved is true.
type ExpectedAlert struct {
	// OrderingID is the number used to sort the slice of expected alerts for a given label set of an alert.
	OrderingID int

	// TimeTolerance is the tolerance to be considered when
	// comparing the time of the alert receiving and alert payload fields.
	// This is usually the group interval.
	// TODO: have some additional tolerance on the http request delay on top of group interval.
	TimeTolerance time.Duration

	// This alert should come at Ts time.
	Ts time.Time

	// NextState is the time when the next state of this alert is expected.
	// time.Time{} if the state won't change from here.
	// This is used to check if this alert can be ignored.
	NextState time.Time

	// If it is a Resolved alert, Resolved must be set to true.
	Resolved bool

	// Resend is true if this alert was a resend of the earlier alert with same labels.
	Resend bool

	// ResolvedTime is the time when the alert becomes Resolved. time.Unix(0,0) if never Resolved.
	// This is also the EndsAt of the alert when the alert is Resolved.
	ResolvedTime time.Time

	// EndsAtDelta is the duration w.r.t. the alert reception time when the EndsAt must be set.
	// This is only for pending and firing alerts.
	// It is usually 4*resendDelay or 4*groupInterval, whichever is higher.
	EndsAtDelta time.Duration

	// This is the expected alert.
	Alert *notifier.Alert
}

// Matches tells if the given alert satisfies the expected alert description.
func (ea *ExpectedAlert) Matches(now time.Time, a notifier.Alert) (err error) {
	if labels.Compare(ea.Alert.Labels, a.Labels) != 0 {
		return fmt.Errorf("labels mismatch, expected: %s, got: %s", ea.Alert.Labels.String(), a.Labels.String())
	}
	if labels.Compare(ea.Alert.Annotations, a.Annotations) != 0 {
		return fmt.Errorf("annotations mismatch, expected: %s, got: %s", ea.Alert.Annotations.String(), a.Annotations.String())
	}

	// TODO: 2*MaxRTT because of some edge case. Like missed by some milli/micro seconds. Fix it.
	if !ea.matchesWithinToleranceAndTwiceSendDelay(ea.Ts, now) {
		return fmt.Errorf("got the alert a little late, expected range: [%s, %s], got: %s",
			ea.Ts.Format(time.RFC3339Nano),
			ea.Ts.Add(ea.TimeTolerance).Format(time.RFC3339Nano),
			now.Format(time.RFC3339Nano),
		)
	}

	if !a.StartsAt.Equal(time.Time{}) && !ea.matchesWithinTolerance(ea.Alert.StartsAt, a.StartsAt) {
		return fmt.Errorf("mismatch in StartsAt, expected range: [%s, %s], got: %s",
			ea.Alert.StartsAt.Format(time.RFC3339Nano),
			ea.Alert.StartsAt.Add(ea.TimeTolerance).Format(time.RFC3339Nano),
			a.StartsAt.Format(time.RFC3339Nano),
		)
	}

	if !a.EndsAt.Equal(time.Time{}) {
		expEndsAt := now.Add(ea.EndsAtDelta)
		if ea.Resolved {
			expEndsAt = ea.ResolvedTime
		}

		// Since EndsAt is w.r.t. the current time for a firing alert, if it does not match the expEndsAt,
		// we need to consider any delay in sending the alert in case the alert was firing.
		if !(ea.matchesWithinTolerance(expEndsAt, a.EndsAt) || ea.matchesWithinTolerance(expEndsAt.Add(-2*MaxRTT), a.EndsAt)) &&
			(ea.Resolved || !ea.matchesWithinTolerance(expEndsAt.Add(-2*MaxRTT), a.EndsAt)) {
			return fmt.Errorf("mismatch in EndsAt, expected range: [%s, %s], got: %s",
				expEndsAt.Format(time.RFC3339Nano),
				expEndsAt.Add(ea.TimeTolerance).Format(time.RFC3339Nano),
				a.EndsAt.Format(time.RFC3339Nano),
			)
		}
	}

	if a.GeneratorURL != "" {
		_, err := url.Parse(a.GeneratorURL)
		if err != nil {
			return fmt.Errorf("generator URL %q does not parse as a URL", a.GeneratorURL)
		}
	}

	return nil
}

func (ea *ExpectedAlert) matchesWithinTolerance(exp, act time.Time) bool {
	return act.After(exp) && act.Before(exp.Add(ea.TimeTolerance))
}

func (ea *ExpectedAlert) matchesWithinToleranceAndTwiceSendDelay(exp, act time.Time) bool {
	return act.After(exp) && act.Before(exp.Add(ea.TimeTolerance+(2*MaxRTT)))
}

func (ea *ExpectedAlert) timeCanBeIgnored(t time.Time) bool {
	// t is within the tolerance OR ea.Ts is after t.
	return ea.matchesWithinToleranceAndTwiceSendDelay(ea.Ts, t) || ea.Ts.After(t)
}

// CanBeIgnored tells if the alert can be ignored. It can be ignored in the following cases:
// 1. It is a firing alert but it gets into "inactive" state within the tolerance time.
// 2. It is a resolved alert but it was resolved more than 15m ago.
// 3. The alert goes into the next state within the tolerance time.
func (ea *ExpectedAlert) CanBeIgnored() bool {
	// TODO: because of time adjusting for resends, this might be wrong.
	return (ea.Resolved && ea.Ts.Sub(ea.ResolvedTime) > 15*time.Minute) || // Time limit for sending resolved.
		// Might have gone into next state.
		(ea.NextState != time.Time{} && ea.timeCanBeIgnored(ea.NextState)) ||
		// Might be near resolved state.
		(ea.ResolvedTime != time.Time{} && !ea.Resolved && ea.timeCanBeIgnored(ea.ResolvedTime))
}

// ShouldBeIgnored tells if the alert should be ignored. It is ignored in the following cases:
// 1. It is a resolved alert and Ts has crossed ResolvedTime+15m.
// 2. Ts has crossed the next state time.
func (ea *ExpectedAlert) ShouldBeIgnored() bool {
	// TODO: because of time adjusting for resends, this might be wrong.
	return (ea.Resolved && ea.Ts.Sub(ea.ResolvedTime) > 15*time.Minute) || // Time limit for sending resolved.
		// Gone into next state.
		(ea.NextState != time.Time{} && ea.Ts.After(ea.NextState))
}
