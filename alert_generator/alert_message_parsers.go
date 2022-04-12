package testsuite

import (
	"encoding/json"
	"github.com/prometheus/alertmanager/notify/webhook"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/notifier"
)

// AlertMessageParsers is a map of the parser name to the parser function.
// The parser name is the one to be used in the config file.
// You can extend this map to include your custom parser and it will be
// matched with the config file.
var AlertMessageParsers = map[string]AlertMessageParser{
	// This parses the alert payload sent by Prometheus.
	"default": func(b []byte) ([]notifier.Alert, error) {
		var alerts []notifier.Alert
		err := json.Unmarshal(b, &alerts)
		return alerts, err
	},

	// This parses the alert payload sent by Prometheus Alertmanager.
	"alertmanager": func(b []byte) ([]notifier.Alert, error) {
		msg := webhook.Message{}
		err := json.Unmarshal(b, &msg)
		if err != nil {
			return nil, err
		}
		alerts := make([]notifier.Alert, 0, len(msg.Alerts))
		for _, al := range msg.Alerts {
			alerts = append(alerts, notifier.Alert{
				Labels:       labels.FromMap(al.Labels),
				Annotations:  labels.FromMap(al.Annotations),
				StartsAt:     al.StartsAt,
				EndsAt:       al.EndsAt,
				GeneratorURL: al.GeneratorURL,
			})
		}
		return alerts, nil
	},
}
