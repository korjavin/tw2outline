package scheduler

import (
	"time"

	"github.com/korjavin/tw2outline/internal/logger"
)

// Scheduler defines the interface for a scheduler.
type Scheduler interface {
	Start()
	Stop()
}

// SimpleScheduler is a basic scheduler that runs a task at a fixed interval.
type SimpleScheduler struct {
	interval time.Duration
	task     func()
	stop     chan struct{}
	logger   *logger.Logger
}

// NewSimpleScheduler creates a new SimpleScheduler.
func NewSimpleScheduler(interval time.Duration, task func(), logger *logger.Logger) *SimpleScheduler {
	return &SimpleScheduler{
		interval: interval,
		task:     task,
		stop:     make(chan struct{}),
		logger:   logger,
	}
}

// Start begins the scheduler's ticking.
func (s *SimpleScheduler) Start() {
	s.logger.Info("Scheduler started, running task every %v", s.interval)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// Run the task once immediately
	s.task()

	for {
		select {
		case <-ticker.C:
			s.task()
		case <-s.stop:
			s.logger.Info("Scheduler stopped")
			return
		}
	}
}

// Stop terminates the scheduler.
func (s *SimpleScheduler) Stop() {
	close(s.stop)
}
