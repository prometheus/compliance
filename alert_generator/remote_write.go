package testsuite

import (
	"context"
	"net/url"
	"sort"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage/remote"
	"go.uber.org/atomic"

	agconfig "github.com/prometheus/compliance/alert_generator/config"
)

func NewRemoteWriter(cfg agconfig.Config, logger log.Logger) (*RemoteWriter, error) {
	u, err := url.Parse(cfg.Settings.RemoteWriteURL)
	if err != nil {
		return nil, err
	}

	var baseAuth *config.BasicAuth
	if cfg.Auth.RemoteWrite.BasicAuthUser != "" {
		baseAuth = &config.BasicAuth{
			Username: cfg.Auth.RemoteWrite.BasicAuthUser,
			Password: config.Secret(cfg.Auth.RemoteWrite.BasicAuthPass),
		}
	}
	client, err := remote.NewWriteClient("alert-generator-test-suite", &remote.ClientConfig{
		URL:              &config.URL{URL: u},
		Timeout:          model.Duration(4 * time.Second),
		RetryOnRateLimit: true,
		HTTPClientConfig: config.HTTPClientConfig{
			BasicAuth: baseAuth,
		},
		SigV4Config: cfg.Auth.RemoteWrite.SigV4Config,
	})
	if err != nil {
		return nil, err
	}
	return &RemoteWriter{
		client: client,
		stopc:  make(chan struct{}),
		errc:   make(chan error, 1),
		log:    log.With(logger, "component", "remote_write"),
	}, nil
}

// RemoteWriter remote writes the time series provided AddTimeSeries()
// in sorted fashion w.r.t. the timestamps.
type RemoteWriter struct {
	client remote.WriteClient

	timeSeries   []prompb.TimeSeries
	allSamples   []sample // Flattened samples from timeSeries.
	totalSamples int

	stopc chan struct{}
	errc  chan error
	err   error
	wg    sync.WaitGroup

	log log.Logger
}

type sample struct {
	labels []prompb.Label
	s      prompb.Sample
}

// AddTimeSeries adds more timeseries to the queue. The timestamp of the samples should be 0 based.
// It should not be called after calling Start().
func (rw *RemoteWriter) AddTimeSeries(ts []prompb.TimeSeries) {
	for _, s := range ts {
		rw.totalSamples += len(s.Samples)
	}
	rw.timeSeries = append(rw.timeSeries, ts...)
}

// Start starts remote-writing the given timeseries. It returns the time corresponding to the 0 timestamp.
func (rw *RemoteWriter) Start() time.Time {
	now := time.Now().UTC()
	nowMs := timestamp.FromTime(now)

	// Flatten all samples from the timeSeries and sort by timestamp.
	rw.allSamples = make([]sample, 0, rw.totalSamples)
	for _, ts := range rw.timeSeries {
		for _, s := range ts.Samples {
			s.Timestamp += nowMs // Making 0 based timestamp relative to the current time.
			rw.allSamples = append(rw.allSamples, sample{
				labels: ts.Labels,
				s:      s,
			})
		}
	}
	sort.Slice(rw.allSamples, func(i, j int) bool {
		return rw.allSamples[i].s.Timestamp < rw.allSamples[j].s.Timestamp
	})

	var timesWritten atomic.Int32
	rw.wg.Add(1)
	go func(allSamples []sample) {
		defer rw.wg.Done()

		var (
			idx int
			buf []byte
			err error
		)

	Outer:
		for idx < len(allSamples) {
			// We wait till it's time for the next sample.
			nextT := allSamples[idx].s.Timestamp
			currT := timestamp.FromTime(time.Now().UTC())
			sleepDuration := time.Duration(nextT-currT) * time.Millisecond

			select {
			case <-rw.stopc:
				break Outer
			case <-time.After(sleepDuration):
				var writeSeries []prompb.TimeSeries
				currT := allSamples[idx].s.Timestamp
				// Batch all samples for this timestamp together.
				// Assumes that at a given timestamp a single series will have only 1 sample.
				for idx < len(allSamples) && allSamples[idx].s.Timestamp == currT {
					writeSeries = append(writeSeries, prompb.TimeSeries{
						Labels:  allSamples[idx].labels,
						Samples: []prompb.Sample{allSamples[idx].s},
					})
					idx++
				}
				buf, err = buildWriteRequest(writeSeries, buf)
				if err != nil {
					rw.errc <- err
					break // TODO: this breaks the select, should actually break the Outer.
				}

				level.Debug(rw.log).Log("msg", "Remote writing", "timestamp", timestamp.Time(currT).Format(time.RFC3339Nano), "total_series", len(writeSeries))
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				err = rw.remoteWrite(ctx, buf)
				timesWritten.Inc()
				if err != nil {
					cancel()
					rw.errc <- err
					level.Debug(rw.log).Log("msg", "Error in remote writing", "timestamp", currT, "total_series", len(writeSeries), "err", err)
					break
				}
				if err := ctx.Err(); err != nil {
					cancel()
					rw.errc <- err
					level.Debug(rw.log).Log("msg", "Error in remote writing", "timestamp", currT, "total_series", len(writeSeries), "err", err)
					break // TODO: fix error handling with this break
				}
				cancel()
			}
		}

	}(rw.allSamples)

	for timesWritten.Load() == 0 {
		time.Sleep(100 * time.Millisecond)
	}

	return now
}

func (rw *RemoteWriter) remoteWrite(ctx context.Context, buf []byte) error {
	err := rw.client.Store(ctx, buf)
	tries := 1
	for err != nil && tries < 3 {
		<-time.After(1 * time.Second)
		tries++
		err = rw.client.Store(ctx, buf)
	}
	return err
}

func (rw *RemoteWriter) Error() error {
	if rw.err != nil {
		return rw.err
	}
	select {
	case rw.err = <-rw.errc:
	default:
	}
	return rw.err
}

func (rw *RemoteWriter) Stop() {
	close(rw.stopc)
}

func (rw *RemoteWriter) Wait() {
	rw.wg.Wait()
}

func buildWriteRequest(ts []prompb.TimeSeries, buf []byte) ([]byte, error) {
	data, err := proto.Marshal(&prompb.WriteRequest{
		Timeseries: ts,
	})
	if err != nil {
		return nil, err
	}

	// snappy uses len() to see if it needs to allocate a new slice. Make the
	// buffer as long as possible.
	if buf != nil {
		buf = buf[0:cap(buf)]
	}
	compressed := snappy.Encode(buf, data)
	return compressed, nil
}
