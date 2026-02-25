package cron

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CronSchedule struct {
	Kind    string `json:"kind"`
	AtMS    *int64 `json:"atMs,omitempty"`
	EveryMS *int64 `json:"everyMs,omitempty"`
	Expr    string `json:"expr,omitempty"`
	TZ      string `json:"tz,omitempty"`
}

type CronPayload struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
	Deliver bool   `json:"deliver"`
	Channel string `json:"channel,omitempty"`
	To      string `json:"to,omitempty"`
}

type CronJobState struct {
	NextRunAtMS *int64 `json:"nextRunAtMs,omitempty"`
	LastRunAtMS *int64 `json:"lastRunAtMs,omitempty"`
	LastStatus  string `json:"lastStatus,omitempty"`
	LastError   string `json:"lastError,omitempty"`
}

type CronJob struct {
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	Enabled        bool         `json:"enabled"`
	Schedule       CronSchedule `json:"schedule"`
	Payload        CronPayload  `json:"payload"`
	State          CronJobState `json:"state"`
	CreatedAtMS    int64        `json:"createdAtMs"`
	UpdatedAtMS    int64        `json:"updatedAtMs"`
	DeleteAfterRun bool         `json:"deleteAfterRun"`
}

type CronStore struct {
	Version int       `json:"version"`
	Jobs    []CronJob `json:"jobs"`
}

type JobHandler func(job *CronJob) (string, error)

type CronService struct {
	storePath    string
	store        *CronStore
	onJob        JobHandler
	mu           sync.RWMutex
	running      bool
	stopChan     chan struct{}
	lastModTime  time.Time // track file modification time to detect external changes
	lastFileSize int64
}

func NewCronService(storePath string, onJob JobHandler) *CronService {
	cs := &CronService{
		storePath: storePath,
		onJob:     onJob,
		stopChan:  make(chan struct{}),
	}
	cs.loadStore()
	return cs
}

func (cs *CronService) Start() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.running {
		return nil
	}

	if err := cs.loadStore(); err != nil {
		return fmt.Errorf("failed to load store: %w", err)
	}

	cs.recomputeNextRuns()
	if err := cs.saveStore(); err != nil {
		return fmt.Errorf("failed to save store: %w", err)
	}

	// Track initial file stats so we can detect external modifications
	if info, err := os.Stat(cs.storePath); err == nil {
		cs.lastModTime = info.ModTime()
		cs.lastFileSize = info.Size()
	}

	cs.running = true
	go cs.runLoop()

	return nil
}

func (cs *CronService) Stop() {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if !cs.running {
		return
	}

	cs.running = false
	close(cs.stopChan)
}

func (cs *CronService) runLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-cs.stopChan:
			return
		case <-ticker.C:
			cs.checkJobs()
		}
	}
}

func (cs *CronService) checkJobs() {
	cs.mu.Lock()
	if !cs.running {
		cs.mu.Unlock()
		return
	}

	// Detect external changes to jobs.json (e.g. from CLI `picoclaw cron add`)
	if info, err := os.Stat(cs.storePath); err == nil {
		if info.ModTime() != cs.lastModTime || info.Size() != cs.lastFileSize {
			cs.loadStore()
			cs.recomputeNextRuns()
			cs.lastModTime = info.ModTime()
			cs.lastFileSize = info.Size()
		}
	}

	now := time.Now().UnixMilli()
	var dueJobs []*CronJob

	for i := range cs.store.Jobs {
		job := &cs.store.Jobs[i]
		if job.Enabled && job.State.NextRunAtMS != nil && *job.State.NextRunAtMS <= now {
			dueJobs = append(dueJobs, job)
		}
	}
	cs.mu.Unlock()

	for _, job := range dueJobs {
		cs.executeJob(job)
	}
}

func (cs *CronService) executeJob(job *CronJob) {
	startTime := time.Now().UnixMilli()

	var err error
	if cs.onJob != nil {
		_, err = cs.onJob(job)
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()

	job.State.LastRunAtMS = &startTime
	job.UpdatedAtMS = time.Now().UnixMilli()

	if err != nil {
		job.State.LastStatus = "error"
		job.State.LastError = err.Error()
	} else {
		job.State.LastStatus = "ok"
		job.State.LastError = ""
	}

	if job.Schedule.Kind == "at" {
		if job.DeleteAfterRun {
			cs.removeJobUnsafe(job.ID)
		} else {
			job.Enabled = false
			job.State.NextRunAtMS = nil
		}
	} else {
		nextRun := cs.computeNextRun(&job.Schedule, time.Now().UnixMilli())
		job.State.NextRunAtMS = nextRun
	}
}

func (cs *CronService) computeNextRun(schedule *CronSchedule, nowMS int64) *int64 {
	if schedule.Kind == "at" {
		if schedule.AtMS != nil && *schedule.AtMS > nowMS {
			return schedule.AtMS
		}
		return nil
	}

	if schedule.Kind == "every" {
		if schedule.EveryMS == nil || *schedule.EveryMS <= 0 {
			return nil
		}
		next := nowMS + *schedule.EveryMS
		return &next
	}

	if schedule.Kind == "cron" && schedule.Expr != "" {
		now := time.UnixMilli(nowMS)
		loc := time.Local
		if schedule.TZ != "" {
			if l, err := time.LoadLocation(schedule.TZ); err == nil {
				loc = l
			}
		}
		now = now.In(loc)
		next := nextCronTime(schedule.Expr, now)
		if next != nil {
			ms := next.UnixMilli()
			return &ms
		}
	}

	return nil
}

// nextCronTime calculates the next time a cron expression should fire after 'after'.
// Supports standard 5-field cron: minute hour day-of-month month day-of-week
// Fields support: numbers, *, */N, ranges (1-5), lists (1,3,5)
func nextCronTime(expr string, after time.Time) *time.Time {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil
	}

	minuteSet := parseCronField(fields[0], 0, 59)
	hourSet := parseCronField(fields[1], 0, 23)
	domSet := parseCronField(fields[2], 1, 31)
	monthSet := parseCronField(fields[3], 1, 12)
	dowSet := parseCronField(fields[4], 0, 6)

	if minuteSet == nil || hourSet == nil || domSet == nil || monthSet == nil || dowSet == nil {
		return nil
	}

	// Start from the next minute
	t := after.Truncate(time.Minute).Add(time.Minute)

	// Search up to 366 days ahead
	limit := after.Add(366 * 24 * time.Hour)
	for t.Before(limit) {
		if monthSet[int(t.Month())] &&
			domSet[t.Day()] &&
			dowSet[int(t.Weekday())] &&
			hourSet[t.Hour()] &&
			minuteSet[t.Minute()] {
			return &t
		}

		// Advance smartly: skip to next valid month/day/hour/minute
		if !monthSet[int(t.Month())] {
			// Jump to first day of next month
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
			continue
		}
		if !domSet[t.Day()] || !dowSet[int(t.Weekday())] {
			// Jump to next day
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
			continue
		}
		if !hourSet[t.Hour()] {
			// Jump to next hour
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, t.Location())
			continue
		}
		// Advance by one minute
		t = t.Add(time.Minute)
	}

	return nil
}

// parseCronField parses a single cron field and returns a set of valid values.
// Supports: * (all), */N (step), N (single), N-M (range), N,M,O (list)
func parseCronField(field string, min, max int) map[int]bool {
	result := make(map[int]bool)

	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)

		// */N - step
		if strings.HasPrefix(part, "*/") {
			step, err := strconv.Atoi(part[2:])
			if err != nil || step <= 0 {
				return nil
			}
			for i := min; i <= max; i += step {
				result[i] = true
			}
			continue
		}

		// * - all
		if part == "*" {
			for i := min; i <= max; i++ {
				result[i] = true
			}
			continue
		}

		// N-M - range
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			lo, err1 := strconv.Atoi(bounds[0])
			hi, err2 := strconv.Atoi(bounds[1])
			if err1 != nil || err2 != nil || lo > hi {
				return nil
			}
			for i := lo; i <= hi; i++ {
				result[i] = true
			}
			continue
		}

		// N - single value
		val, err := strconv.Atoi(part)
		if err != nil {
			return nil
		}
		result[val] = true
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

func (cs *CronService) recomputeNextRuns() {
	now := time.Now().UnixMilli()
	for i := range cs.store.Jobs {
		job := &cs.store.Jobs[i]
		if job.Enabled {
			job.State.NextRunAtMS = cs.computeNextRun(&job.Schedule, now)
		}
	}
}

func (cs *CronService) getNextWakeMS() *int64 {
	var nextWake *int64
	for _, job := range cs.store.Jobs {
		if job.Enabled && job.State.NextRunAtMS != nil {
			if nextWake == nil || *job.State.NextRunAtMS < *nextWake {
				nextWake = job.State.NextRunAtMS
			}
		}
	}
	return nextWake
}

func (cs *CronService) Load() error {
	return cs.loadStore()
}

func (cs *CronService) loadStore() error {
	cs.store = &CronStore{
		Version: 1,
		Jobs:    []CronJob{},
	}

	data, err := os.ReadFile(cs.storePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return json.Unmarshal(data, cs.store)
}

func (cs *CronService) saveStore() error {
	dir := filepath.Dir(cs.storePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cs.store, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(cs.storePath, data, 0644); err != nil {
		return err
	}

	// Track file stats after our own write so we can detect external changes
	if info, err := os.Stat(cs.storePath); err == nil {
		cs.lastModTime = info.ModTime()
		cs.lastFileSize = info.Size()
	}

	return nil
}

func (cs *CronService) AddJob(name string, schedule CronSchedule, message string, deliver bool, channel, to string) (*CronJob, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	now := time.Now().UnixMilli()

	job := CronJob{
		ID:       generateID(),
		Name:     name,
		Enabled:  true,
		Schedule: schedule,
		Payload: CronPayload{
			Kind:    "agent_turn",
			Message: message,
			Deliver: deliver,
			Channel: channel,
			To:      to,
		},
		State: CronJobState{
			NextRunAtMS: cs.computeNextRun(&schedule, now),
		},
		CreatedAtMS:    now,
		UpdatedAtMS:    now,
		DeleteAfterRun: false,
	}

	cs.store.Jobs = append(cs.store.Jobs, job)
	if err := cs.saveStore(); err != nil {
		return nil, err
	}

	return &job, nil
}

func (cs *CronService) RemoveJob(jobID string) bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	return cs.removeJobUnsafe(jobID)
}

func (cs *CronService) removeJobUnsafe(jobID string) bool {
	before := len(cs.store.Jobs)
	var jobs []CronJob
	for _, job := range cs.store.Jobs {
		if job.ID != jobID {
			jobs = append(jobs, job)
		}
	}
	cs.store.Jobs = jobs
	removed := len(cs.store.Jobs) < before

	if removed {
		cs.saveStore()
	}

	return removed
}

func (cs *CronService) EnableJob(jobID string, enabled bool) *CronJob {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	for i := range cs.store.Jobs {
		job := &cs.store.Jobs[i]
		if job.ID == jobID {
			job.Enabled = enabled
			job.UpdatedAtMS = time.Now().UnixMilli()

			if enabled {
				job.State.NextRunAtMS = cs.computeNextRun(&job.Schedule, time.Now().UnixMilli())
			} else {
				job.State.NextRunAtMS = nil
			}

			cs.saveStore()
			return job
		}
	}

	return nil
}

func (cs *CronService) ListJobs(includeDisabled bool) []CronJob {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	if includeDisabled {
		return cs.store.Jobs
	}

	var enabled []CronJob
	for _, job := range cs.store.Jobs {
		if job.Enabled {
			enabled = append(enabled, job)
		}
	}

	return enabled
}

func (cs *CronService) Status() map[string]interface{} {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	var enabledCount int
	for _, job := range cs.store.Jobs {
		if job.Enabled {
			enabledCount++
		}
	}

	return map[string]interface{}{
		"enabled":      cs.running,
		"jobs":         len(cs.store.Jobs),
		"nextWakeAtMS": cs.getNextWakeMS(),
	}
}

func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
