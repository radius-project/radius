/*
Copyright 2026 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package hashmigration runs the automatic migration of values that older versions of Radius
// derived with SHA-1 to their SHA-256 form.
//
// Each control-plane component migrates the data it owns in the background. A pass never blocks
// the component from serving requests: values that cannot be migrated yet (for example because a
// Terraform state is locked) are retried on a later pass, and every runtime lookup keeps its SHA-1
// fallback until a later release removes it. Progress is recorded in the ConfigMap named
// ConfigMapName so that operators, and the release that removes the SHA-1 fallback, can verify
// that the migration finished. See https://github.com/radius-project/radius/issues/8084.
package hashmigration

import (
	"context"
	"fmt"
	"time"

	"github.com/radius-project/radius/pkg/components/hosting"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"github.com/radius-project/radius/pkg/version"
)

const (
	// ConfigMapName is the ConfigMap in the Radius namespace that records the migration status of every step.
	ConfigMapName = "radius-sha256-migration"

	// DefaultNamespace is the namespace that holds the status ConfigMap and the migrated Kubernetes objects.
	DefaultNamespace = "radius-system"

	// DefaultInitialDelay lets the component finish starting before the first pass.
	DefaultInitialDelay = 30 * time.Second

	// DefaultRetryInterval is the delay before retrying a pass that left work behind.
	DefaultRetryInterval = time.Minute

	// DefaultMaxRetryInterval caps the exponential retry delay.
	DefaultMaxRetryInterval = 15 * time.Minute
)

// Config controls the migration for a component.
type Config struct {
	// Disabled turns the automatic migration off. It exists for in-process test hosts; production installations must
	// leave it enabled because a later release requires the migration to have completed.
	Disabled bool `yaml:"disabled"`
}

// State is the state of one migration step.
type State string

const (
	// StateInProgress means legacy values remain and will be retried.
	StateInProgress State = "InProgress"

	// StateComplete means no legacy values remain. Conflicts and skipped values are reported but do not block completion.
	StateComplete State = "Complete"

	// StateNotApplicable means the step does not apply to this installation, for example because its data store does
	// not derive names from hashes.
	StateNotApplicable State = "NotApplicable"

	// StateFailed means the last pass returned an error. The step is retried.
	StateFailed State = "Failed"
)

// Result is the outcome of one pass of a Step.
type Result struct {
	// Migrated counts values moved or rewritten to their SHA-256 form during the pass.
	Migrated int `json:"migrated"`

	// Deduplicated counts legacy copies that were removed because an identical current copy already existed.
	Deduplicated int `json:"deduplicated,omitempty"`

	// Remaining counts legacy values that could not be migrated during the pass, and work the step waits for, such as
	// pods of an earlier release that can still write legacy values. They are retried.
	Remaining int `json:"remaining"`

	// Conflicts counts values that exist under both their legacy and current names with different content. The
	// current copy is the one Radius reads; the legacy copy is left in place for review rather than deleted.
	Conflicts int `json:"conflicts,omitempty"`

	// Skipped counts values that cannot be migrated automatically, for example because their origin cannot be
	// verified. They are reported and not retried.
	Skipped int `json:"skipped,omitempty"`

	// NotApplicable marks a step that does not apply to this installation.
	NotApplicable bool `json:"-"`

	// Message carries an optional human-readable detail.
	Message string `json:"message,omitempty"`
}

// Add accumulates the counts of other into r.
func (r *Result) Add(other Result) {
	r.Migrated += other.Migrated
	r.Deduplicated += other.Deduplicated
	r.Remaining += other.Remaining
	r.Conflicts += other.Conflicts
	r.Skipped += other.Skipped
}

// Step migrates one kind of value. Run must be idempotent and safe to run concurrently with the component's normal
// operations and with other replicas running the same step.
type Step interface {
	// Name returns the stable name used in the status ConfigMap key.
	Name() string

	// Run performs one pass.
	Run(ctx context.Context) (Result, error)
}

// StepFunc adapts a function to the Step interface.
type StepFunc struct {
	// StepName is returned by Name.
	StepName string

	// Fn performs one pass.
	Fn func(ctx context.Context) (Result, error)
}

// Name implements Step.
func (s StepFunc) Name() string {
	return s.StepName
}

// Run implements Step.
func (s StepFunc) Run(ctx context.Context) (Result, error) {
	return s.Fn(ctx)
}

// Status is the recorded status of one step. Migrated and Deduplicated count every value the component migrated since
// it started; Remaining, Conflicts, Skipped, and Message describe the last pass.
type Status struct {
	Result

	// State is the state after the last pass.
	State State `json:"state"`

	// Version is the Radius version that recorded the status.
	Version string `json:"version"`

	// Updated is when the status was recorded.
	Updated time.Time `json:"updated"`

	// Error is the error returned by the last pass, if any.
	Error string `json:"error,omitempty"`
}

// Done reports whether the step needs no further passes.
func (s Status) Done() bool {
	return s.State == StateComplete || s.State == StateNotApplicable
}

// StatusKey returns the ConfigMap key that records the status of step in component.
func StatusKey(component string, step string) string {
	return component + "." + step
}

// Options configures a Service.
type Options struct {
	// Component is the name of the component running the migration, such as ucp or applications-rp.
	Component string

	// Steps run in order during every pass.
	Steps []Step

	// Reporter records step status. A nil Reporter only logs.
	Reporter Reporter

	// InitialDelay is the delay before the first pass. Zero means no delay.
	InitialDelay time.Duration

	// RetryInterval is the delay before the first retry. Zero means DefaultRetryInterval.
	RetryInterval time.Duration

	// MaxRetryInterval caps the retry delay. Zero means DefaultMaxRetryInterval.
	MaxRetryInterval time.Duration
}

// Service runs migration passes until every step is done. It implements hosting.Service and returns nil once the
// migration is complete, which stops the service without stopping the host.
type Service struct {
	options Options
	now     func() time.Time

	// totals accumulates the results of each step across passes.
	totals map[string]Result

	// finished holds the final status of each step that needs no further passes. Such a step is not run again.
	finished map[string]Status

	// recorded records that the final status of a step was recorded by the Reporter.
	recorded map[string]bool
}

var _ hosting.Service = (*Service)(nil)

// NewService creates a Service.
func NewService(options Options) *Service {
	if options.RetryInterval <= 0 {
		options.RetryInterval = DefaultRetryInterval
	}
	if options.MaxRetryInterval <= 0 {
		options.MaxRetryInterval = DefaultMaxRetryInterval
	}
	if options.MaxRetryInterval < options.RetryInterval {
		options.MaxRetryInterval = options.RetryInterval
	}

	return &Service{options: options, now: time.Now, totals: map[string]Result{}, finished: map[string]Status{}, recorded: map[string]bool{}}
}

// Name implements hosting.Service.
func (s *Service) Name() string {
	return fmt.Sprintf("%s sha256 migration", s.options.Component)
}

// Run implements hosting.Service.
func (s *Service) Run(ctx context.Context) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	// A status recorded before this process started can be stale. For example, after a rollback to an earlier release
	// that writes legacy values and an upgrade back, the recorded status still says Complete until this process has run
	// the step again.
	s.reportPending(ctx)

	if !sleep(ctx, s.options.InitialDelay) {
		return ctx.Err()
	}

	interval := s.options.RetryInterval
	for pass := 1; ; pass++ {
		if s.RunPass(ctx) {
			logger.Info("SHA-256 migration is complete", "component", s.options.Component, "passes", pass)
			return nil
		}

		logger.Info("SHA-256 migration will retry", "component", s.options.Component, "pass", pass, "delay", interval.String())
		if !sleep(ctx, interval) {
			return ctx.Err()
		}

		interval = min(interval*2, s.options.MaxRetryInterval)
	}
}

// pendingMessage is the message of the status that records a step as in progress until this process runs it.
const pendingMessage = "waiting for the first pass of the migration after the component started"

// reportPending records every step as in progress until this process runs it. It is best effort: every pass records
// the status of each step again, and the migration is not complete until those statuses are recorded.
func (s *Service) reportPending(ctx context.Context) {
	if s.options.Reporter == nil {
		return
	}

	logger := ucplog.FromContextOrDiscard(ctx)
	for _, step := range s.options.Steps {
		status := Status{
			Result:  Result{Message: pendingMessage},
			State:   StateInProgress,
			Version: version.Version(),
			Updated: s.now().UTC(),
		}
		if err := s.options.Reporter.Report(ctx, StatusKey(s.options.Component, step.Name()), status); err != nil {
			logger.Info("Unable to record SHA-256 migration status", "component", s.options.Component, "step", step.Name(), "error", err.Error())
		}
	}
}

// RunPass runs every step that is not done, records the status of each, and reports whether all steps are done and
// their final status is recorded. A step that is done is not run again. If its final status could not be recorded, a
// later pass records it, because operators and the release that removes the SHA-1 fallback rely on the status.
func (s *Service) RunPass(ctx context.Context) bool {
	logger := ucplog.FromContextOrDiscard(ctx)
	done := true
	for _, step := range s.options.Steps {
		name := step.Name()
		status, finished := s.finished[name]
		if finished && s.recorded[name] {
			continue
		}

		if !finished {
			status = s.runStep(ctx, step)
			values := []any{"component", s.options.Component, "step", name, "state", status.State,
				"migrated", status.Migrated, "deduplicated", status.Deduplicated, "remaining", status.Remaining,
				"conflicts", status.Conflicts, "skipped", status.Skipped}
			if status.Error != "" {
				logger.Info("SHA-256 migration step failed", append(values, "error", status.Error)...)
			} else {
				logger.Info("SHA-256 migration step finished", values...)
			}

			if status.Done() {
				s.finished[name] = status
			} else {
				done = false
			}
		}

		if s.options.Reporter == nil {
			continue
		}

		if err := s.options.Reporter.Report(ctx, StatusKey(s.options.Component, name), status); err != nil {
			// The migration is not complete until its status is recorded, so a later pass records it.
			logger.Info("Unable to record SHA-256 migration status; a later pass records it", "component", s.options.Component, "step", name, "error", err.Error())
			done = false
			continue
		}

		if status.Done() {
			s.recorded[name] = true
		}
	}

	return done
}

func (s *Service) runStep(ctx context.Context, step Step) (status Status) {
	status = Status{Version: version.Version(), Updated: s.now().UTC()}
	defer func() {
		if value := recover(); value != nil {
			status.State = StateFailed
			status.Error = fmt.Sprintf("step panicked: %v", value)
		}
	}()

	result, err := step.Run(ctx)
	total := s.totals[step.Name()]
	total.Migrated += result.Migrated
	total.Deduplicated += result.Deduplicated
	total.Remaining, total.Conflicts, total.Skipped = result.Remaining, result.Conflicts, result.Skipped
	total.NotApplicable, total.Message = result.NotApplicable, result.Message
	s.totals[step.Name()] = total
	status.Result = total

	switch {
	case err != nil:
		status.State = StateFailed
		status.Error = err.Error()
	case result.NotApplicable:
		status.State = StateNotApplicable
	case result.Remaining > 0:
		status.State = StateInProgress
	default:
		status.State = StateComplete
	}

	return status
}

// sleep waits for d or until ctx is done, and reports whether the full duration elapsed.
func sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}

	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
