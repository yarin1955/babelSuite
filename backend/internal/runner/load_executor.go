package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/babelsuite/babelsuite/internal/logstream"
	"github.com/babelsuite/babelsuite/internal/suites"
)

const maxOutstandingLoadRequests = 32

type loadSamplerStats struct {
	Count     int
	Failures  int
	Latencies []time.Duration
}

type loadStageStats struct {
	Index         int
	Users         int
	SpawnRate     float64
	Planned       time.Duration
	Stop          bool
	StartedAt     time.Time
	FinishedAt    time.Time
	Requests      int
	Failures      int
	Latencies     []time.Duration
	ThroughputByS map[int]int
}

type loadStats struct {
	mu            sync.Mutex
	StartedAt     time.Time
	FinishedAt    time.Time
	Requests      int
	Failures      int
	PeakUsers     int
	Latencies     []time.Duration
	ThroughputByS map[int]int
	Samplers      map[string]*loadSamplerStats
	Stages        map[int]*loadStageStats
}

func executeLoadStep(ctx context.Context, step StepSpec, emit func(logstream.Line)) error {
	if step.Load == nil {
		return fmt.Errorf("traffic step %q is missing a parsed traffic spec", step.Node.Name)
	}
	if step.Node.Variant == "traffic.scalability" {
		return fmt.Errorf("traffic profile %q requires distributed execution and is not supported by the native runner", step.Node.Variant)
	}

	stats := &loadStats{
		StartedAt:     time.Now(),
		ThroughputByS: make(map[int]int),
		Samplers:      make(map[string]*loadSamplerStats),
		Stages:        make(map[int]*loadStageStats),
	}
	emit(line(step, "info", fmt.Sprintf("[%s] Loaded native traffic plan %s against %s.", step.Node.Name, step.Load.PlanPath, step.Load.Target)))
	if useSyntheticLoadTarget(step.Load.Target) {
		emit(line(step, "info", fmt.Sprintf("[%s] Target %s resolves as a suite-local symbolic service; using bounded synthetic traffic.", step.Node.Name, step.Load.Target)))
		if err := runSyntheticLoadModel(ctx, step, emit, stats); err != nil {
			return err
		}
		return finalizeLoadStep(step, emit, stats)
	}

	// Delegate to APISIX sidecar when the environment provides a gateway URL.
	if canUseAPISIXTraffic(step) {
		if err := runAPISIXTraffic(ctx, step, emit, stats); err != nil {
			return err
		}
		return finalizeLoadStep(step, emit, stats)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	switch step.Node.Variant {
	case "traffic.constant_throughput", "traffic.open_model":
		if err := runOpenLoadModel(ctx, step, client, emit, stats); err != nil {
			return err
		}
	default:
		if err := runClosedLoadModel(ctx, step, client, emit, stats); err != nil {
			return err
		}
	}

	return finalizeLoadStep(step, emit, stats)
}

func finalizeLoadStep(step StepSpec, emit func(logstream.Line), stats *loadStats) error {
	if err := evaluateLoadThresholds(step, stats); err != nil {
		return err
	}

	summary := stats.summary()
	emit(line(step, "info", fmt.Sprintf("[%s] Native traffic run completed with %d requests, %d failures, and peak concurrency %d.", step.Node.Name, summary.Requests, summary.Failures, summary.PeakUsers)))
	emit(line(step, "info", fmt.Sprintf("[%s] Latency avg=%.1fms min=%.1fms max=%.1fms p50=%.1fms p90=%.1fms p95=%.1fms p99=%.1fms.", step.Node.Name, summary.Latency.AvgMillis, summary.Latency.MinMillis, summary.Latency.MaxMillis, summary.Latency.P50Millis, summary.Latency.P90Millis, summary.Latency.P95Millis, summary.Latency.P99Millis)))
	emit(line(step, "info", fmt.Sprintf("[%s] Throughput avg=%.1frps peak=%.1frps timeline=%s.", step.Node.Name, summary.AverageRPS, summary.PeakRPS, formatThroughputTimeline(summary.Throughput))))
	if histogram := formatHistogram(summary.Latency.Histogram); histogram != "" {
		emit(line(step, "info", fmt.Sprintf("[%s] Latency histogram %s.", step.Node.Name, histogram)))
	}
	for _, sampler := range summary.Samplers {
		emit(line(step, "info", fmt.Sprintf("[%s] Sampler %s avg=%.1fms min=%.1fms max=%.1fms p50=%.1fms p90=%.1fms p95=%.1fms p99=%.1fms count=%d failures=%d.", step.Node.Name, sampler.Name, sampler.Latency.AvgMillis, sampler.Latency.MinMillis, sampler.Latency.MaxMillis, sampler.Latency.P50Millis, sampler.Latency.P90Millis, sampler.Latency.P95Millis, sampler.Latency.P99Millis, sampler.Count, sampler.Failures)))
	}
	for _, stage := range summary.Stages {
		emit(line(step, "info", fmt.Sprintf("[%s] Stage %d summary users=%d spawn_rate=%.1f duration=%s requests=%d failures=%d avg_rps=%.1f peak_rps=%.1f avg=%.1fms p95=%.1fms p99=%.1fms.", step.Node.Name, stage.Index+1, stage.Users, stage.SpawnRate, stage.ActualDuration, stage.Requests, stage.Failures, stage.AverageRPS, stage.PeakRPS, stage.Latency.AvgMillis, stage.Latency.P95Millis, stage.Latency.P99Millis)))
	}
	return nil
}

func useSyntheticLoadTarget(target string) bool {
	parsed, err := url.Parse(strings.TrimSpace(target))
	if err != nil {
		return false
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") || net.ParseIP(host) != nil {
		return false
	}
	return !strings.Contains(host, ".")
}

func runSyntheticLoadModel(ctx context.Context, step StepSpec, emit func(logstream.Line), stats *loadStats) error {
	selector := rand.New(rand.NewSource(time.Now().UnixNano()))
	for stageIndex, stage := range step.Load.Stages {
		stats.beginStage(stageIndex, stage, time.Now())
		emit(line(step, "info", fmt.Sprintf("[%s] Entering stage %d with target users=%d spawn_rate=%.1f duration=%s.", step.Node.Name, stageIndex+1, stage.Users, stage.SpawnRate, stage.Duration)))
		if err := runSyntheticStage(ctx, step, stageIndex, stage, selector, stats); err != nil {
			return err
		}
		stats.endStage(stageIndex, time.Now())
		if stage.Stop {
			break
		}
	}
	return nil
}

func runSyntheticStage(ctx context.Context, step StepSpec, stageIndex int, stage suites.LoadStage, selector *rand.Rand, stats *loadStats) error {
	stageSeconds := int(math.Ceil(stage.Duration.Seconds()))
	if stageSeconds <= 0 {
		stageSeconds = 1
	}
	stats.recordActiveUsers(stage.Users)

	for tick := 0; tick < stageSeconds; tick++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		switch step.Node.Variant {
		case "traffic.constant_throughput", "traffic.open_model":
			rate := stageLoadRate(step.Load, stage)
			iterations := maxInt(1, int(math.Ceil(rate)))
			recordSyntheticIterations(step.Load.Users, selector, iterations, stageIndex, stats)
		default:
			iterations := maxInt(1, stage.Users)
			recordSyntheticIterations(step.Load.Users, selector, iterations, stageIndex, stats)
		}

		wait := time.Second
		if remaining := stage.Duration - (time.Duration(tick+1) * time.Second); remaining < 0 && remaining > -time.Second {
			wait = time.Second + remaining
		}
		if tick == stageSeconds-1 || wait <= 0 {
			continue
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}

	return nil
}

func recordSyntheticIterations(users []suites.LoadUser, selector *rand.Rand, iterations int, stageIndex int, stats *loadStats) {
	if len(users) == 0 {
		stats.recordResult("synthetic", 45*time.Millisecond, false, stageIndex, time.Now())
		return
	}
	for index := 0; index < iterations; index++ {
		user := pickLoadUser(users, selector)
		if len(user.Tasks) == 0 {
			stats.recordResult(firstNonEmpty(user.Name, "synthetic"), 45*time.Millisecond, false, stageIndex, time.Now())
			continue
		}
		task := pickLoadTask(user.Tasks, selector)
		latency := syntheticTaskLatency(taskSamplerName(task))
		stats.recordResult(taskSamplerName(task), latency, false, stageIndex, time.Now())
	}
}

func syntheticTaskLatency(name string) time.Duration {
	var total int
	for _, ch := range name {
		total += int(ch)
	}
	return time.Duration(35+(total%25)) * time.Millisecond
}

func runClosedLoadModel(ctx context.Context, step StepSpec, client *http.Client, emit func(logstream.Line), stats *loadStats) error {
	active := make([]loadUserHandle, 0)
	defer stopLoadUsers(&active)

	selector := rand.New(rand.NewSource(time.Now().UnixNano()))
	for stageIndex, stage := range step.Load.Stages {
		stats.beginStage(stageIndex, stage, time.Now())
		emit(line(step, "info", fmt.Sprintf("[%s] Entering stage %d with target users=%d spawn_rate=%.1f duration=%s.", step.Node.Name, stageIndex+1, stage.Users, stage.SpawnRate, stage.Duration)))
		stageCtx, cancel := context.WithTimeout(ctx, stage.Duration)
		ticker := time.NewTicker(time.Second)
		tick := 0

		adjustClosedUsers(stageCtx, stageIndex, step, client, emit, stats, &active, stage, selector)
	stageLoop:
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				cancel()
				return ctx.Err()
			case <-stageCtx.Done():
				break stageLoop
			case <-ticker.C:
				adjustClosedUsers(stageCtx, stageIndex, step, client, emit, stats, &active, stage, selector)
				tick++
				if tick%5 == 0 {
					emit(metricLine(step, snapshotMetrics(stats)))
				}
			}
		}

		ticker.Stop()
		cancel()
		stopLoadUsers(&active)
		stats.endStage(stageIndex, time.Now())
		if stage.Stop {
			break
		}
	}
	return nil
}

type loadUserHandle struct {
	cancel context.CancelFunc
	done   chan struct{}
}

func adjustClosedUsers(ctx context.Context, stageIndex int, step StepSpec, client *http.Client, emit func(logstream.Line), stats *loadStats, active *[]loadUserHandle, stage suites.LoadStage, selector *rand.Rand) {
	target := stage.Users
	diff := target - len(*active)
	if diff == 0 {
		stats.recordActiveUsers(len(*active))
		return
	}

	stepSize := len(*active) + 1
	if stage.SpawnRate > 0 {
		stepSize = int(math.Ceil(stage.SpawnRate))
	}
	if stepSize <= 0 {
		stepSize = 1
	}

	switch {
	case diff > 0:
		toStart := minInt(diff, stepSize)
		for index := 0; index < toStart; index++ {
			user := pickLoadUser(step.Load.Users, selector)
			*active = append(*active, startLoadUser(ctx, stageIndex, step, user, client, emit, stats, selector.Int()))
		}
	case diff < 0:
		toStop := minInt(-diff, stepSize)
		for index := 0; index < toStop && len(*active) > 0; index++ {
			handle := (*active)[len(*active)-1]
			handle.cancel()
			<-handle.done
			*active = (*active)[:len(*active)-1]
		}
	}

	stats.recordActiveUsers(len(*active))
}

func startLoadUser(ctx context.Context, stageIndex int, step StepSpec, user suites.LoadUser, client *http.Client, emit func(logstream.Line), stats *loadStats, seed int) loadUserHandle {
	userCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		rnd := rand.New(rand.NewSource(time.Now().UnixNano() + int64(seed)))
		for {
			select {
			case <-userCtx.Done():
				return
			default:
			}

			iterationStartedAt := time.Now()
			task := pickLoadTask(user.Tasks, rnd)
			runLoadTask(userCtx, stageIndex, step, client, task, stats)

			wait := loadUserWait(user.Wait, iterationStartedAt, rnd)
			if wait <= 0 {
				continue
			}
			timer := time.NewTimer(wait)
			select {
			case <-userCtx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
	return loadUserHandle{cancel: cancel, done: done}
}

func stopLoadUsers(active *[]loadUserHandle) {
	for index := len(*active) - 1; index >= 0; index-- {
		(*active)[index].cancel()
		<-(*active)[index].done
	}
	*active = (*active)[:0]
}

func runOpenLoadModel(ctx context.Context, step StepSpec, client *http.Client, emit func(logstream.Line), stats *loadStats) error {
	limiter := make(chan struct{}, maxOutstandingLoadRequests)
	var wg sync.WaitGroup
	selector := rand.New(rand.NewSource(time.Now().UnixNano()))
	defer wg.Wait()

	for stageIndex, stage := range step.Load.Stages {
		rate := stageLoadRate(step.Load, stage)
		if rate <= 0 {
			continue
		}
		stats.beginStage(stageIndex, stage, time.Now())
		emit(line(step, "info", fmt.Sprintf("[%s] Entering stage %d with rate %.1f iterations/s for %s.", step.Node.Name, stageIndex+1, rate, stage.Duration)))

		interval := time.Duration(float64(time.Second) / rate)
		if interval < 10*time.Millisecond {
			interval = 10 * time.Millisecond
		}

		stageCtx, cancel := context.WithTimeout(ctx, stage.Duration)
		ticker := time.NewTicker(interval)
		metricTicker := time.NewTicker(5 * time.Second)
	stageLoop:
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				metricTicker.Stop()
				cancel()
				return ctx.Err()
			case <-stageCtx.Done():
				break stageLoop
			case <-metricTicker.C:
				emit(metricLine(step, snapshotMetrics(stats)))
			case <-ticker.C:
				select {
				case limiter <- struct{}{}:
					stats.recordActiveUsers(len(limiter))
					user := pickLoadUser(step.Load.Users, selector)
					task := pickLoadTask(user.Tasks, selector)
					wg.Add(1)
					go func(selected suites.LoadTask, selectedStage int) {
						defer wg.Done()
						defer func() {
							<-limiter
							stats.recordActiveUsers(len(limiter))
						}()
						runLoadTask(stageCtx, selectedStage, step, client, selected, stats)
					}(task, stageIndex)
				default:
				}
			}
		}
		ticker.Stop()
		metricTicker.Stop()
		cancel()
		stats.endStage(stageIndex, time.Now())
		if stage.Stop {
			break
		}
	}
	return nil
}

func runLoadTask(ctx context.Context, stageIndex int, step StepSpec, client *http.Client, task suites.LoadTask, stats *loadStats) {
	if ctx.Err() != nil {
		return
	}

	requestURL, err := resolveLoadTaskURL(step.Load.Target, task.Request.Path)
	if err != nil {
		stats.recordResult(taskSamplerName(task), 0, true, stageIndex, time.Now())
		return
	}

	request, err := http.NewRequestWithContext(ctx, task.Request.Method, requestURL, strings.NewReader(task.Request.Body))
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		stats.recordResult(taskSamplerName(task), 0, true, stageIndex, time.Now())
		return
	}
	for key, value := range task.Request.Headers {
		request.Header.Set(key, value)
	}

	startedAt := time.Now()
	response, err := client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		stats.recordResult(taskSamplerName(task), time.Since(startedAt), true, stageIndex, time.Now())
		return
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()

	failed := !loadStatusChecksPass(task.Checks, response.StatusCode)
	stats.recordResult(taskSamplerName(task), time.Since(startedAt), failed, stageIndex, time.Now())
}

func resolveLoadTaskURL(target string, path string) (string, error) {
	base, err := url.Parse(target)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(ref).String(), nil
}

func loadStatusChecksPass(checks []suites.LoadThreshold, statusCode int) bool {
	for _, check := range checks {
		if check.Metric != "status" {
			continue
		}
		if !compareLoadValue(float64(statusCode), check.Op, check.Value) {
			return false
		}
	}
	return true
}

func evaluateLoadThresholds(step StepSpec, stats *loadStats) error {
	thresholds := make([]suites.LoadThreshold, 0, len(step.Load.Thresholds)+4)
	thresholds = append(thresholds, step.Load.Thresholds...)
	if !containsLoadMetric(thresholds, "http.error_rate") {
		thresholds = append(thresholds, suites.LoadThreshold{Metric: "http.error_rate", Op: "<=", Value: 0})
	}
	for _, user := range step.Load.Users {
		for _, task := range user.Tasks {
			for _, check := range task.Checks {
				if check.Metric == "status" {
					continue
				}
				normalized := check
				if normalized.Sampler == "" {
					normalized.Sampler = taskSamplerName(task)
				}
				thresholds = append(thresholds, normalized)
			}
		}
	}

	failures := make([]string, 0)
	summary := stats.summary()
	for _, threshold := range thresholds {
		switch threshold.Metric {
		case "http.error_rate":
			if !compareLoadValue(summary.ErrorRate, threshold.Op, threshold.Value) {
				failures = append(failures, fmt.Sprintf("%s %s %.2f failed (got %.4f)", threshold.Metric, threshold.Op, threshold.Value, summary.ErrorRate))
			}
		case "http.min_ms", "latency.min_ms", "http.avg_ms", "latency.avg_ms", "http.max_ms", "latency.max_ms", "http.p50_ms", "latency.p50_ms", "http.p90_ms", "latency.p90_ms", "http.p95_ms", "latency.p95_ms", "http.p99_ms", "latency.p99_ms", "throughput.avg_rps", "traffic.avg_rps", "throughput.peak_rps", "traffic.peak_rps":
			value, ok := summary.metricValue(threshold.Metric, threshold.Sampler)
			if !ok || !compareLoadValue(value, threshold.Op, threshold.Value) {
				failures = append(failures, fmt.Sprintf("%s %s %.2f failed for %s", threshold.Metric, threshold.Op, threshold.Value, firstNonEmpty(threshold.Sampler, "all")))
			}
		}
	}
	if len(failures) == 0 {
		return nil
	}
	return errors.New(strings.Join(failures, "; "))
}

type loadSummary struct {
	Requests       int
	Failures       int
	ErrorRate      float64
	PeakUsers      int
	ActualDuration time.Duration
	AverageRPS     float64
	PeakRPS        float64
	Latency        loadLatencySummary
	Throughput     []loadThroughputPoint
	Samplers       []loadSamplerSummary
	Stages         []loadStageSummary
}

type loadSamplerSummary struct {
	Name      string
	Count     int
	Failures  int
	ErrorRate float64
	Latency   loadLatencySummary
}

type loadStageSummary struct {
	Index           int
	Users           int
	SpawnRate       float64
	PlannedDuration time.Duration
	ActualDuration  time.Duration
	Requests        int
	Failures        int
	ErrorRate       float64
	AverageRPS      float64
	PeakRPS         float64
	Latency         loadLatencySummary
}

type loadThroughputPoint struct {
	OffsetSeconds int
	Requests      int
}

type loadLatencySummary struct {
	Count     int
	MinMillis float64
	MaxMillis float64
	AvgMillis float64
	P50Millis float64
	P90Millis float64
	P95Millis float64
	P99Millis float64
	Histogram []loadHistogramBucket
}

type loadHistogramBucket struct {
	Label string
	Count int
}

func (s *loadStats) beginStage(index int, stage suites.LoadStage, startedAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current := s.Stages[index]
	if current == nil {
		current = &loadStageStats{
			Index:         index,
			ThroughputByS: make(map[int]int),
		}
		s.Stages[index] = current
	}
	current.Users = stage.Users
	current.SpawnRate = stage.SpawnRate
	current.Planned = stage.Duration
	current.Stop = stage.Stop
	current.StartedAt = startedAt
	if current.FinishedAt.Before(startedAt) {
		current.FinishedAt = time.Time{}
	}
}

func (s *loadStats) endStage(index int, finishedAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current := s.Stages[index]
	if current == nil {
		return
	}
	current.FinishedAt = finishedAt
	if finishedAt.After(s.FinishedAt) {
		s.FinishedAt = finishedAt
	}
}

func (s *loadStats) recordResult(sampler string, latency time.Duration, failed bool, stageIndex int, observedAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.StartedAt.IsZero() || observedAt.Before(s.StartedAt) {
		s.StartedAt = observedAt
	}
	if observedAt.After(s.FinishedAt) {
		s.FinishedAt = observedAt
	}
	s.Requests++
	if failed {
		s.Failures++
	}
	s.Latencies = append(s.Latencies, latency)
	s.ThroughputByS[throughputBucketIndex(s.StartedAt, observedAt)]++
	current := s.Samplers[sampler]
	if current == nil {
		current = &loadSamplerStats{}
		s.Samplers[sampler] = current
	}
	current.Count++
	if failed {
		current.Failures++
	}
	current.Latencies = append(current.Latencies, latency)

	stage := s.Stages[stageIndex]
	if stage == nil {
		stage = &loadStageStats{
			Index:         stageIndex,
			ThroughputByS: make(map[int]int),
		}
		s.Stages[stageIndex] = stage
	}
	if stage.StartedAt.IsZero() || observedAt.Before(stage.StartedAt) {
		stage.StartedAt = observedAt
	}
	if observedAt.After(stage.FinishedAt) {
		stage.FinishedAt = observedAt
	}
	stage.Requests++
	if failed {
		stage.Failures++
	}
	stage.Latencies = append(stage.Latencies, latency)
	stage.ThroughputByS[throughputBucketIndex(stage.StartedAt, observedAt)]++
}

func (s *loadStats) recordActiveUsers(count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if count > s.PeakUsers {
		s.PeakUsers = count
	}
}

func (s *loadStats) summary() loadSummary {
	s.mu.Lock()
	defer s.mu.Unlock()

	summary := loadSummary{
		Requests:   s.Requests,
		Failures:   s.Failures,
		PeakUsers:  s.PeakUsers,
		Latency:    summarizeLatencies(s.Latencies),
		Throughput: summarizeThroughput(s.ThroughputByS),
		Samplers:   make([]loadSamplerSummary, 0, len(s.Samplers)),
		Stages:     make([]loadStageSummary, 0, len(s.Stages)),
	}
	if s.Requests > 0 {
		summary.ErrorRate = float64(s.Failures) / float64(s.Requests)
	}
	if !s.StartedAt.IsZero() && !s.FinishedAt.IsZero() && s.FinishedAt.After(s.StartedAt) {
		summary.ActualDuration = s.FinishedAt.Sub(s.StartedAt)
	}
	summary.AverageRPS, summary.PeakRPS = summarizeRPS(summary.Requests, summary.ActualDuration, summary.Throughput)
	for name, sampler := range s.Samplers {
		errorRate := 0.0
		if sampler.Count > 0 {
			errorRate = float64(sampler.Failures) / float64(sampler.Count)
		}
		summary.Samplers = append(summary.Samplers, loadSamplerSummary{
			Name:      name,
			Count:     sampler.Count,
			Failures:  sampler.Failures,
			ErrorRate: errorRate,
			Latency:   summarizeLatencies(sampler.Latencies),
		})
	}
	sort.Slice(summary.Samplers, func(i, j int) bool {
		return summary.Samplers[i].Name < summary.Samplers[j].Name
	})
	for index, stage := range s.Stages {
		actualDuration := stage.Planned
		if !stage.StartedAt.IsZero() && !stage.FinishedAt.IsZero() && stage.FinishedAt.After(stage.StartedAt) {
			actualDuration = stage.FinishedAt.Sub(stage.StartedAt)
		}
		errorRate := 0.0
		if stage.Requests > 0 {
			errorRate = float64(stage.Failures) / float64(stage.Requests)
		}
		points := summarizeThroughput(stage.ThroughputByS)
		avgRPS, peakRPS := summarizeRPS(stage.Requests, actualDuration, points)
		summary.Stages = append(summary.Stages, loadStageSummary{
			Index:           index,
			Users:           stage.Users,
			SpawnRate:       stage.SpawnRate,
			PlannedDuration: stage.Planned,
			ActualDuration:  actualDuration,
			Requests:        stage.Requests,
			Failures:        stage.Failures,
			ErrorRate:       errorRate,
			AverageRPS:      avgRPS,
			PeakRPS:         peakRPS,
			Latency:         summarizeLatencies(stage.Latencies),
		})
	}
	sort.Slice(summary.Stages, func(i, j int) bool {
		return summary.Stages[i].Index < summary.Stages[j].Index
	})
	return summary
}

// snapshotMetrics reads the current loadStats and returns a TrafficMetricSnapshot
// suitable for emission as a metric-kind log line.
func snapshotMetrics(stats *loadStats) TrafficMetricSnapshot {
	s := stats.summary()
	return TrafficMetricSnapshot{
		Requests:  s.Requests,
		Failures:  s.Failures,
		ErrorRate: s.ErrorRate,
		RPS:       s.AverageRPS,
		Users:     s.PeakUsers,
		MinMS:     s.Latency.MinMillis,
		AvgMS:     s.Latency.AvgMillis,
		P50MS:     s.Latency.P50Millis,
		P95MS:     s.Latency.P95Millis,
		P99MS:     s.Latency.P99Millis,
		MaxMS:     s.Latency.MaxMillis,
	}
}

func (s loadSummary) metricValue(metric string, sampler string) (float64, bool) {
	if metric == "http.error_rate" {
		return s.ErrorRate, true
	}
	if metric == "throughput.avg_rps" || metric == "traffic.avg_rps" {
		return s.AverageRPS, true
	}
	if metric == "throughput.peak_rps" || metric == "traffic.peak_rps" {
		return s.PeakRPS, true
	}

	latency := s.Latency
	if sampler != "" {
		found := false
		for _, item := range s.Samplers {
			if item.Name == sampler {
				latency = item.Latency
				found = true
				break
			}
		}
		if !found {
			return 0, false
		}
	}

	switch metric {
	case "http.min_ms", "latency.min_ms":
		return latency.MinMillis, latency.Count > 0
	case "http.avg_ms", "latency.avg_ms":
		return latency.AvgMillis, latency.Count > 0
	case "http.max_ms", "latency.max_ms":
		return latency.MaxMillis, latency.Count > 0
	case "http.p50_ms", "latency.p50_ms":
		return latency.P50Millis, latency.Count > 0
	case "http.p90_ms", "latency.p90_ms":
		return latency.P90Millis, latency.Count > 0
	case "http.p95_ms", "latency.p95_ms":
		return latency.P95Millis, latency.Count > 0
	case "http.p99_ms", "latency.p99_ms":
		return latency.P99Millis, latency.Count > 0
	default:
		return 0, false
	}
}

func summarizeLatencies(values []time.Duration) loadLatencySummary {
	if len(values) == 0 {
		return loadLatencySummary{}
	}
	minimum := values[0]
	maximum := values[0]
	total := time.Duration(0)
	for _, value := range values {
		if value < minimum {
			minimum = value
		}
		if value > maximum {
			maximum = value
		}
		total += value
	}
	return loadLatencySummary{
		Count:     len(values),
		MinMillis: durationMillis(minimum),
		MaxMillis: durationMillis(maximum),
		AvgMillis: durationMillis(total) / float64(len(values)),
		P50Millis: percentileMillis(values, 50),
		P90Millis: percentileMillis(values, 90),
		P95Millis: percentileMillis(values, 95),
		P99Millis: percentileMillis(values, 99),
		Histogram: buildLatencyHistogram(values),
	}
}

func summarizeThroughput(buckets map[int]int) []loadThroughputPoint {
	if len(buckets) == 0 {
		return nil
	}
	offsets := make([]int, 0, len(buckets))
	for offset := range buckets {
		offsets = append(offsets, offset)
	}
	sort.Ints(offsets)
	points := make([]loadThroughputPoint, 0, len(offsets))
	for _, offset := range offsets {
		points = append(points, loadThroughputPoint{
			OffsetSeconds: offset,
			Requests:      buckets[offset],
		})
	}
	return points
}

func summarizeRPS(requests int, duration time.Duration, throughput []loadThroughputPoint) (float64, float64) {
	peak := 0.0
	for _, point := range throughput {
		if float64(point.Requests) > peak {
			peak = float64(point.Requests)
		}
	}
	if duration <= 0 {
		return peak, peak
	}
	return float64(requests) / duration.Seconds(), peak
}

func percentileMillis(values []time.Duration, percentile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]time.Duration{}, values...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})
	position := int(math.Ceil((percentile / 100) * float64(len(sorted))))
	if position <= 0 {
		position = 1
	}
	if position > len(sorted) {
		position = len(sorted)
	}
	return durationMillis(sorted[position-1])
}

func durationMillis(value time.Duration) float64 {
	return float64(value) / float64(time.Millisecond)
}

func throughputBucketIndex(startedAt time.Time, observedAt time.Time) int {
	if startedAt.IsZero() || observedAt.Before(startedAt) {
		return 0
	}
	return int(observedAt.Sub(startedAt) / time.Second)
}

func buildLatencyHistogram(values []time.Duration) []loadHistogramBucket {
	thresholds := []struct {
		label string
		limit time.Duration
	}{
		{label: "<=10ms", limit: 10 * time.Millisecond},
		{label: "<=25ms", limit: 25 * time.Millisecond},
		{label: "<=50ms", limit: 50 * time.Millisecond},
		{label: "<=100ms", limit: 100 * time.Millisecond},
		{label: "<=250ms", limit: 250 * time.Millisecond},
		{label: "<=500ms", limit: 500 * time.Millisecond},
		{label: "<=1000ms", limit: time.Second},
	}
	buckets := make([]loadHistogramBucket, 0, len(thresholds)+1)
	for _, threshold := range thresholds {
		buckets = append(buckets, loadHistogramBucket{Label: threshold.label})
	}
	buckets = append(buckets, loadHistogramBucket{Label: ">1000ms"})
	for _, value := range values {
		matched := false
		for index, threshold := range thresholds {
			if value <= threshold.limit {
				buckets[index].Count++
				matched = true
				break
			}
		}
		if !matched {
			buckets[len(buckets)-1].Count++
		}
	}
	return buckets
}

func formatThroughputTimeline(points []loadThroughputPoint) string {
	if len(points) == 0 {
		return "none"
	}
	parts := make([]string, 0, minInt(len(points), 12))
	appendPoint := func(point loadThroughputPoint) {
		parts = append(parts, fmt.Sprintf("t+%ds=%drps", point.OffsetSeconds, point.Requests))
	}
	if len(points) <= 12 {
		for _, point := range points {
			appendPoint(point)
		}
		return strings.Join(parts, ", ")
	}
	for _, point := range points[:8] {
		appendPoint(point)
	}
	parts = append(parts, "...")
	for _, point := range points[len(points)-3:] {
		appendPoint(point)
	}
	return strings.Join(parts, ", ")
}

func formatHistogram(buckets []loadHistogramBucket) string {
	if len(buckets) == 0 {
		return ""
	}
	parts := make([]string, 0, len(buckets))
	for _, bucket := range buckets {
		parts = append(parts, fmt.Sprintf("%s=%d", bucket.Label, bucket.Count))
	}
	return strings.Join(parts, ", ")
}

func pickLoadUser(users []suites.LoadUser, selector *rand.Rand) suites.LoadUser {
	total := 0
	for _, user := range users {
		total += maxInt(1, user.Weight)
	}
	if total <= 0 {
		return users[0]
	}
	target := selector.Intn(total)
	cursor := 0
	for _, user := range users {
		cursor += maxInt(1, user.Weight)
		if target < cursor {
			return user
		}
	}
	return users[len(users)-1]
}

func pickLoadTask(tasks []suites.LoadTask, selector *rand.Rand) suites.LoadTask {
	total := 0
	for _, task := range tasks {
		total += maxInt(1, task.Weight)
	}
	if total <= 0 {
		return tasks[0]
	}
	target := selector.Intn(total)
	cursor := 0
	for _, task := range tasks {
		cursor += maxInt(1, task.Weight)
		if target < cursor {
			return task
		}
	}
	return tasks[len(tasks)-1]
}

func loadUserWait(wait suites.LoadWait, iterationStartedAt time.Time, selector *rand.Rand) time.Duration {
	switch wait.Mode {
	case "between":
		minimum := wait.MinSeconds
		maximum := wait.MaxSeconds
		if maximum < minimum {
			maximum = minimum
		}
		return time.Duration((minimum + selector.Float64()*(maximum-minimum)) * float64(time.Second))
	case "pacing":
		target := time.Duration(wait.Seconds * float64(time.Second))
		elapsed := time.Since(iterationStartedAt)
		if elapsed >= target {
			return 0
		}
		return target - elapsed
	default:
		return time.Duration(wait.Seconds * float64(time.Second))
	}
}

func stageLoadRate(spec *suites.LoadSpec, stage suites.LoadStage) float64 {
	switch spec.Variant {
	case "traffic.constant_throughput":
		if spec.RequestsPerS > 0 {
			return spec.RequestsPerS
		}
	case "traffic.open_model":
		if spec.ArrivalRate > 0 {
			return spec.ArrivalRate
		}
	}
	return float64(stage.Users)
}

func taskSamplerName(task suites.LoadTask) string {
	return firstNonEmpty(task.Request.Name, task.Name, task.Request.Path)
}

func compareLoadValue(actual float64, op string, expected float64) bool {
	switch op {
	case "==":
		return actual == expected
	case "!=":
		return actual != expected
	case "<":
		return actual < expected
	case "<=":
		return actual <= expected
	case ">":
		return actual > expected
	case ">=":
		return actual >= expected
	default:
		return false
	}
}

func containsLoadMetric(thresholds []suites.LoadThreshold, metric string) bool {
	for _, threshold := range thresholds {
		if threshold.Metric == metric {
			return true
		}
	}
	return false
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
