package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mtui/mapcleaner"
	"mtui/types"
	"os"
	"path"
	"sync"
	"time"

	"github.com/luanti-golang/areasparser"
	"github.com/sirupsen/logrus"
)

// MapCleaner manages the background removal of unprotected map chunks
type MapCleaner struct {
	app    *App
	mu     sync.Mutex
	state  *mapcleaner.State
	cancel context.CancelFunc
	done   chan struct{}
}

// MapCleanerStatus is the current state with some derived values
type MapCleanerStatus struct {
	*mapcleaner.State
	TotalChunks int64 `json:"total_chunks"`
}

func NewMapCleaner(a *App) *MapCleaner {
	return &MapCleaner{app: a}
}

func (m *MapCleaner) loadJSON(key types.ConfigKey, v any) (bool, error) {
	e, err := m.app.Repos.ConfigRepo.GetByKey(key)
	if err != nil {
		return false, err
	}
	if e == nil || e.Value == "" {
		return false, nil
	}
	return true, json.Unmarshal([]byte(e.Value), v)
}

func (m *MapCleaner) saveJSON(key types.ConfigKey, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return m.app.Repos.ConfigRepo.Set(&types.ConfigEntry{Key: key, Value: string(data)})
}

// GetSettings returns the stored settings or the defaults
func (m *MapCleaner) GetSettings() (*mapcleaner.Settings, error) {
	s := mapcleaner.DefaultSettings()
	_, err := m.loadJSON(types.ConfigMapcleanerSettings, s)
	return s, err
}

// SetSettings validates and stores the settings, the progress is reset if the range changes
func (m *MapCleaner) SetSettings(s *mapcleaner.Settings) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cancel != nil {
		return errors.New("settings can't be changed while the cleanup is running")
	}

	s.Normalize()
	err := s.Validate()
	if err != nil {
		return err
	}

	old, err := m.GetSettings()
	if err != nil {
		return err
	}

	err = m.saveJSON(types.ConfigMapcleanerSettings, s)
	if err != nil {
		return err
	}

	if !old.SameRange(s) {
		return m.resetState(s)
	}
	return nil
}

// must be called with the lock held
func (m *MapCleaner) getState() (*mapcleaner.State, error) {
	if m.state != nil {
		return m.state, nil
	}

	s, err := m.GetSettings()
	if err != nil {
		return nil, err
	}

	state := mapcleaner.NewState(s)
	_, err = m.loadJSON(types.ConfigMapcleanerState, state)
	if err != nil {
		return nil, err
	}
	m.state = state
	return state, nil
}

// must be called with the lock held
func (m *MapCleaner) saveState(state *mapcleaner.State) error {
	state.UpdatedAt = time.Now().UnixMilli()
	m.state = state
	return m.saveJSON(types.ConfigMapcleanerState, state)
}

// must be called with the lock held
func (m *MapCleaner) resetState(s *mapcleaner.Settings) error {
	return m.saveState(mapcleaner.NewState(s))
}

func (m *MapCleaner) Status() (*MapCleanerStatus, error) {
	s, err := m.GetSettings()
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	state, err := m.getState()
	if err != nil {
		return nil, err
	}
	snapshot := *state
	return &MapCleanerStatus{State: &snapshot, TotalChunks: s.TotalChunks()}, nil
}

func (m *MapCleaner) loadAreas() ([]*areasparser.Area, error) {
	for _, filename := range []string{"areas.dat", "areas.json"} {
		p := path.Join(m.app.WorldDir, filename)
		_, err := os.Stat(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		areas, err := areasparser.ParseFile(p)
		if err != nil {
			return nil, fmt.Errorf("error parsing '%s': %v", filename, err)
		}
		return areas, nil
	}
	return nil, errors.New("areas protection is enabled but no 'areas.dat' or 'areas.json' was found in the world directory")
}

// Start starts a new run or resumes a paused one
func (m *MapCleaner) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cancel != nil {
		return errors.New("the cleanup is already running")
	}

	if m.app.DBContext == nil || m.app.DBContext.Blocks == nil {
		return errors.New("no map database available")
	}

	s, err := m.GetSettings()
	if err != nil {
		return err
	}
	s.Normalize()
	err = s.Validate()
	if err != nil {
		return err
	}

	var areas []*areasparser.Area
	if s.UseAreas {
		areas, err = m.loadAreas()
		if err != nil {
			return err
		}
	}

	current, err := m.getState()
	if err != nil {
		return err
	}

	var state *mapcleaner.State
	resumable := current.Status == mapcleaner.StatusPaused || current.Status == mapcleaner.StatusError
	if resumable && current.DryRun == s.DryRun {
		// continue at the last position
		snapshot := *current
		state = &snapshot
	} else {
		state = mapcleaner.NewState(s)
		state.StartedAt = time.Now().UnixMilli()
		state.DryRun = s.DryRun
	}
	state.Status = mapcleaner.StatusRunning
	state.Error = ""

	err = m.saveState(state)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.done = make(chan struct{})

	cleaner := mapcleaner.New(m.app.DBContext.Blocks, s, areas)
	runState := *state
	go m.run(ctx, cleaner, &runState, m.done)

	logrus.WithFields(logrus.Fields{
		"dry_run":   s.DryRun,
		"processed": state.Processed,
	}).Info("mapcleaner: started")
	return nil
}

func (m *MapCleaner) run(ctx context.Context, cleaner *mapcleaner.Cleaner, state *mapcleaner.State, done chan struct{}) {
	defer close(done)

	err := cleaner.Run(ctx, state, func(s mapcleaner.State, persist bool) error {
		m.mu.Lock()
		defer m.mu.Unlock()
		if persist {
			return m.saveState(&s)
		}
		s.UpdatedAt = time.Now().UnixMilli()
		m.state = &s
		return nil
	})

	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancel = nil

	var msg string
	switch {
	case err == nil:
		state.Status = mapcleaner.StatusDone
		msg = fmt.Sprintf("Map cleanup finished: %d chunks processed, %d removed, %d retained", state.Processed, state.Removed, state.Retained)
	case errors.Is(err, context.Canceled):
		state.Status = mapcleaner.StatusPaused
	default:
		state.Status = mapcleaner.StatusError
		state.Error = err.Error()
		msg = fmt.Sprintf("Map cleanup failed at chunk %d,%d,%d: %v", state.ChunkX, state.ChunkY, state.ChunkZ, err)
		logrus.WithError(err).Error("mapcleaner: run failed")
	}
	if state.DryRun && msg != "" {
		msg = "[dry-run] " + msg
	}

	err = m.saveState(state)
	if err != nil {
		logrus.WithError(err).Error("mapcleaner: could not save state")
	}

	if msg != "" && m.app.Repos != nil {
		m.app.CreateUILogEntry(&types.Log{Event: "mapcleaner", Message: msg}, nil)
	}
}

// Pause stops the current run and waits until the state is saved
func (m *MapCleaner) Pause() error {
	m.mu.Lock()
	if m.cancel == nil {
		m.mu.Unlock()
		return errors.New("the cleanup is not running")
	}
	m.cancel()
	done := m.done
	m.mu.Unlock()

	<-done
	return nil
}

// Reset discards the progress
func (m *MapCleaner) Reset() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cancel != nil {
		return errors.New("the cleanup is running, pause it first")
	}

	s, err := m.GetSettings()
	if err != nil {
		return err
	}
	return m.resetState(s)
}

// ResumeInterrupted continues a run that was interrupted by a restart of mtui
func (m *MapCleaner) ResumeInterrupted() error {
	m.mu.Lock()
	state, err := m.getState()
	if err != nil {
		m.mu.Unlock()
		return err
	}
	if state.Status != mapcleaner.StatusRunning {
		m.mu.Unlock()
		return nil
	}
	// mark as paused, the position of the last z-stride is persisted
	state.Status = mapcleaner.StatusPaused
	err = m.saveState(state)
	m.mu.Unlock()
	if err != nil {
		return err
	}

	f, err := m.app.Repos.FeatureRepository.GetByName(types.FEATURE_MAPCLEANER)
	if err != nil {
		return err
	}
	if f == nil || !f.Enabled {
		return nil
	}

	logrus.Info("mapcleaner: resuming interrupted run")
	return m.Start()
}
