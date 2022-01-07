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
)

func NewRemoteWriter(rwURL string, logger log.Logger) (*RemoteWriter, error) {
	u, err := url.Parse(rwURL)
	if err != nil {
		return nil, err
	}
	client, err := remote.NewWriteClient("alert-generator-test-suite", &remote.ClientConfig{
		URL:              &config.URL{URL: u},
		Timeout:          model.Duration(1 * time.Second),
		RetryOnRateLimit: true,
	})
	if err != nil {
		return nil, err
	}
	return &RemoteWriter{
		client: client,
		stopc:  make(chan struct{}),
		errc:   make(chan error, 1),
		log:    logger,
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
					break
				}

				level.Debug(rw.log).Log("msg", "Remote writing", "timestamp", currT, "total_series", len(writeSeries))
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				err = rw.client.Store(ctx, buf)
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
					break
				}
				cancel()
			}
		}

	}(rw.allSamples)

	return now
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
