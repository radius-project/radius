# Prometheus Metrics Usage


1. For number of requests, use value as 1 for each request:

	metric.Must(global.GetMeterProvider().Meter("radius-rp")).NewInt64ValueRecorder(requestMetricName, metric.WithUnit(unit.Dimensionless)).Record(r.Context(), int64(1))

2. To record latency:

	metric.Must(global.GetMeterProvider().Meter("radius-rp")).NewInt64Counter(metricName, metric.WithUnit(unit.millisecond)).Add(ctx, int64(val), labels...)

3. To use a guage, define a call back function:

	callback := func(v int) metric.Int64ObserverFunc {
		return metric.Int64ObserverFunc(func(_ context.Context, result metric.Int64ObserverResult) { result.Observe(int64(v), labels...) })
	}(val)

	getMeterMust().NewInt64ValueObserver(metricName, callback).Observation(int64(val))