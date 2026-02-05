// Package discovery provides application discovery and analysis functionality.
package discovery

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// Progress tracks and displays progress for long-running operations.
type Progress struct {
	mu          sync.Mutex
	writer      io.Writer
	total       int
	current     int
	message     string
	startTime   time.Time
	showSpinner bool
	spinnerIdx  int
	done        bool
	stopCh      chan struct{}
}

// ProgressOptions configures progress display.
type ProgressOptions struct {
	// Writer is where to write progress output.
	Writer io.Writer

	// Total is the total number of items to process (0 for indeterminate).
	Total int

	// ShowSpinner enables animated spinner for indeterminate progress.
	ShowSpinner bool
}

// NewProgress creates a new Progress tracker.
func NewProgress(opts ProgressOptions) *Progress {
	if opts.Writer == nil {
		opts.Writer = io.Discard
	}

	return &Progress{
		writer:      opts.Writer,
		total:       opts.Total,
		showSpinner: opts.ShowSpinner,
		startTime:   time.Now(),
		stopCh:      make(chan struct{}),
	}
}

// Start begins progress tracking with an optional spinner animation.
func (p *Progress) Start(message string) {
	p.mu.Lock()
	p.message = message
	p.startTime = time.Now()
	p.mu.Unlock()

	if p.showSpinner {
		go p.runSpinner()
	} else {
		p.render()
	}
}

// Update updates the progress with current count and message.
func (p *Progress) Update(current int, message string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.current = current
	if message != "" {
		p.message = message
	}

	if !p.showSpinner {
		p.render()
	}
}

// Increment increases the current count by 1.
func (p *Progress) Increment(message string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.current++
	if message != "" {
		p.message = message
	}

	if !p.showSpinner {
		p.render()
	}
}

// Complete marks the progress as complete.
func (p *Progress) Complete(message string) {
	p.mu.Lock()
	p.done = true
	if message != "" {
		p.message = message
	}
	p.mu.Unlock()

	close(p.stopCh)

	// Final render
	p.renderComplete()
}

// Fail marks the progress as failed.
func (p *Progress) Fail(message string) {
	p.mu.Lock()
	p.done = true
	if message != "" {
		p.message = message
	}
	p.mu.Unlock()

	close(p.stopCh)

	// Final render with failure indicator
	p.renderFail()
}

func (p *Progress) runSpinner() {
	spinnerChars := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.mu.Lock()
			p.spinnerIdx = (p.spinnerIdx + 1) % len(spinnerChars)
			spinner := spinnerChars[p.spinnerIdx]
			message := p.message
			current := p.current
			total := p.total
			p.mu.Unlock()

			// Clear line and render
			fmt.Fprintf(p.writer, "\r\033[K%s %s", spinner, p.formatProgress(current, total, message))
		}
	}
}

func (p *Progress) render() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.total > 0 {
		// Determinate progress
		percentage := float64(p.current) / float64(p.total) * 100
		bar := p.renderProgressBar(p.current, p.total, 30)
		fmt.Fprintf(p.writer, "\r\033[K[%s] %.1f%% %s", bar, percentage, p.message)
	} else {
		// Indeterminate progress
		fmt.Fprintf(p.writer, "\r\033[K%s", p.message)
	}
}

func (p *Progress) renderComplete() {
	elapsed := time.Since(p.startTime)

	p.mu.Lock()
	message := p.message
	p.mu.Unlock()

	fmt.Fprintf(p.writer, "\r\033[K✓ %s (%.2fs)\n", message, elapsed.Seconds())
}

func (p *Progress) renderFail() {
	elapsed := time.Since(p.startTime)

	p.mu.Lock()
	message := p.message
	p.mu.Unlock()

	fmt.Fprintf(p.writer, "\r\033[K✗ %s (%.2fs)\n", message, elapsed.Seconds())
}

func (p *Progress) formatProgress(current, total int, message string) string {
	if total > 0 {
		percentage := float64(current) / float64(total) * 100
		return fmt.Sprintf("[%d/%d] %.1f%% %s", current, total, percentage, message)
	}
	return message
}

func (p *Progress) renderProgressBar(current, total, width int) string {
	if total == 0 {
		return strings.Repeat("-", width)
	}

	filled := int(float64(current) / float64(total) * float64(width))
	if filled > width {
		filled = width
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return bar
}

// ProgressGroup tracks multiple progress items.
type ProgressGroup struct {
	mu       sync.Mutex
	writer   io.Writer
	items    []*ProgressItem
	complete int
}

// ProgressItem represents a single progress item in a group.
type ProgressItem struct {
	Name     string
	Status   ProgressStatus
	Message  string
	Duration time.Duration
}

// ProgressStatus represents the status of a progress item.
type ProgressStatus int

const (
	// StatusPending indicates the item has not started.
	StatusPending ProgressStatus = iota
	// StatusRunning indicates the item is in progress.
	StatusRunning
	// StatusComplete indicates the item completed successfully.
	StatusComplete
	// StatusFailed indicates the item failed.
	StatusFailed
	// StatusSkipped indicates the item was skipped.
	StatusSkipped
)

// NewProgressGroup creates a new ProgressGroup.
func NewProgressGroup(writer io.Writer) *ProgressGroup {
	if writer == nil {
		writer = io.Discard
	}
	return &ProgressGroup{
		writer: writer,
	}
}

// Add adds a new item to the progress group.
func (g *ProgressGroup) Add(name string) *ProgressItem {
	g.mu.Lock()
	defer g.mu.Unlock()

	item := &ProgressItem{
		Name:   name,
		Status: StatusPending,
	}
	g.items = append(g.items, item)
	return item
}

// Start marks an item as running.
func (g *ProgressGroup) Start(item *ProgressItem) {
	g.mu.Lock()
	defer g.mu.Unlock()

	item.Status = StatusRunning
	g.render()
}

// Complete marks an item as complete.
func (g *ProgressGroup) Complete(item *ProgressItem, duration time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()

	item.Status = StatusComplete
	item.Duration = duration
	g.complete++
	g.render()
}

// Fail marks an item as failed.
func (g *ProgressGroup) Fail(item *ProgressItem, message string, duration time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()

	item.Status = StatusFailed
	item.Message = message
	item.Duration = duration
	g.complete++
	g.render()
}

// Skip marks an item as skipped.
func (g *ProgressGroup) Skip(item *ProgressItem, reason string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	item.Status = StatusSkipped
	item.Message = reason
	g.complete++
	g.render()
}

func (g *ProgressGroup) render() {
	// Simple render - just print the current state
	for _, item := range g.items {
		var statusIcon string
		switch item.Status {
		case StatusPending:
			statusIcon = "○"
		case StatusRunning:
			statusIcon = "◐"
		case StatusComplete:
			statusIcon = "✓"
		case StatusFailed:
			statusIcon = "✗"
		case StatusSkipped:
			statusIcon = "⊘"
		}

		if item.Duration > 0 {
			fmt.Fprintf(g.writer, "%s %s (%.2fs)\n", statusIcon, item.Name, item.Duration.Seconds())
		} else {
			fmt.Fprintf(g.writer, "%s %s\n", statusIcon, item.Name)
		}
	}
}

// Summary returns a summary of the progress group.
func (g *ProgressGroup) Summary() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	var completed, failed, skipped int
	for _, item := range g.items {
		switch item.Status {
		case StatusComplete:
			completed++
		case StatusFailed:
			failed++
		case StatusSkipped:
			skipped++
		}
	}

	total := len(g.items)
	return fmt.Sprintf("%d/%d completed, %d failed, %d skipped", completed, total, failed, skipped)
}
