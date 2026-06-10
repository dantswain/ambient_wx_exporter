package metrics

import (
	"fmt"
	"time"

	"github.com/dantswain/ambient_wx_exporter/pkg/ambient_wx_exporter/state"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/lrosenman/ambient"
	"github.com/prometheus/client_golang/prometheus"
)

var apiKeysWithoutMetrics = map[string]bool{
	"dateutc":  true,
	"date":     true,
	"tz":       true,
	"lastRain": true,
}

func getAmbientDevices(key ambient.Key, logger log.Logger) (ambient.APIDeviceResponse, error) {
	var dr ambient.APIDeviceResponse
	maxRetries := 5
	backoff := 1 * time.Second
	for attempt := 1; attempt <= maxRetries; attempt++ {
		var err error
		dr, err = ambient.Device(key)
		if err != nil {
			level.Warn(logger).Log(
				"msg", "Ambient API request failed",
				"attempt", attempt,
				"error", err,
			)
		} else {
			switch dr.HTTPResponseCode {
			case 200:
				level.Info(logger).Log("msg", "Successfully fetched data from Ambient API")
				return dr, nil
			case 429, 502, 503:
				level.Warn(logger).Log(
					"msg", "Recoverable HTTP error from Ambient API. Retrying.",
					"attempt", attempt,
					"status_code", dr.HTTPResponseCode,
				)
			default:
				level.Error(logger).Log(
					"msg", "Unrecoverable HTTP error from Ambient API.",
					"status_code", dr.HTTPResponseCode,
				)
				return dr, fmt.Errorf("unrecoverable ambient API status %d", dr.HTTPResponseCode)
			}
		}

		if attempt == maxRetries {
			break
		}

		time.Sleep(backoff)
		backoff *= 2
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
	}

	return dr, fmt.Errorf("failed to fetch ambient devices after %d attempts", maxRetries)
}

func recordDeviceMetrics(device ambient.DeviceRecord, theState *state.State, logger log.Logger) {
	for k, v := range device.LastDataFields {
		if apiKeysWithoutMetrics[k] {
			continue
		}
		gauges, ok := theState.Gauges[device.Macaddress]
		if ok {
			gauge, ok := gauges[k]
			if ok {
				labels := theState.Labels[device.Macaddress][k]
				setGaugeInterface(gauge, labels, v, logger)
			} else {
				level.Debug(logger).Log("msg", "No config for ambient metric", "mac_address", device.Macaddress, "api_key", k)
			}
		} else {
			level.Warn(logger).Log("msg", "No config for mac address", "mac_address", device.Macaddress)
		}
	}
}

func recordAges(device ambient.DeviceRecord, theState *state.State, logger log.Logger) {
	datetime, ok := device.LastDataFields["dateutc"]
	now := time.Now()
	if ok {
		millis := now.UnixNano() / 1000000.0
		diff := (float64(millis) - (datetime.(float64))) / 1000.0
		theState.DataAgeGauge.With(
			prometheus.Labels{"mac_address": device.Macaddress},
		).Set(diff)
	}

	raintime, ok := device.LastDataFields["lastRain"]
	if ok {
		layout := "2006-01-02T15:04:05.000Z"
		t, err := time.Parse(layout, raintime.(string))

		if err != nil {
			level.Warn(logger).Log(
				"msg", "Unable to parse last rain timestamp",
				"timestamp", raintime.(string),
				"error", err,
			)
		}

		diff := float64(now.Unix() - t.Unix())
		theState.RainAgeGauge.With(
			prometheus.Labels{"mac_address": device.Macaddress},
		).Set(diff)
	}
}

func setGaugeInterface(gauge *prometheus.GaugeVec, labels map[string]string, val interface{}, logger log.Logger) {
	switch v := val.(type) {
	// case string:
	// 	gauge.With(labels).Set(0.0)
	case int64:
		gauge.With(labels).Set(float64(v))
	case float64:
		gauge.With(labels).Set(v)
	default:
		level.Warn(logger).Log("msg", "Unhandled metric type", "type", fmt.Sprintf("%T", v))
	}
}

func recordDefaultMetrics(device ambient.DeviceRecord, theState *state.State, logger log.Logger) {
	labels := map[string]string{"mac_address": device.Macaddress}
	for k, v := range device.LastDataFields {
		if apiKeysWithoutMetrics[k] {
			continue
		}
		gauge, ok := theState.DefaultGauges[k]
		if ok {
			setGaugeInterface(gauge, labels, v, logger)
		} else {
			level.Warn(logger).Log("msg", "No default metric defined for api key", "mac_address", device.Macaddress, "api_key", k)
		}
	}
}

func recordLoop(key ambient.Key, theState *state.State, logger log.Logger) {
	dr, err := getAmbientDevices(key, logger)
	if err != nil {
		level.Error(logger).Log("msg", "Unable to fetch Ambient device data", "error", err)
		return
	}

	for _, device := range dr.DeviceRecord {
		level.Info(logger).Log("msg", "Recording device metrics", "mac_address", device.Macaddress)
		if len(theState.Gauges) > 0 {
			recordDeviceMetrics(device, theState, logger)
		}
		recordAges(device, theState, logger)
		if len(theState.DefaultGauges) > 0 {
			recordDefaultMetrics(device, theState, logger)
		}
	}
}

// RecordMetrics spawns a loop to record metrics
func RecordMetrics(theState *state.State, logger log.Logger) {
	key := ambient.NewKey(theState.AppKey, theState.APIKey)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				level.Error(logger).Log("msg", "Recovered from metrics worker panic", "error", r)
			}
		}()
		for {
			recordLoop(key, theState, logger)
			time.Sleep(60 * time.Second)
		}
	}()
}
