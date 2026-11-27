package app_test

import (
	"mtui/app"
	"mtui/mapcleaner"
	"mtui/testutil"
	"os"
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMapCleanerLifecycle(t *testing.T) {
	a := testutil.NewApp(t)
	m := a.MapCleaner

	// defaults
	s, err := m.GetSettings()
	assert.NoError(t, err)
	assert.True(t, s.DryRun)

	st, err := m.Status()
	assert.NoError(t, err)
	assert.Equal(t, mapcleaner.StatusIdle, st.Status)
	assert.Equal(t, s.TotalChunks(), st.TotalChunks)

	// invalid settings are rejected
	s.UseAreas = false
	assert.Error(t, m.SetSettings(s))

	// areas enabled but no areas file
	s.UseAreas = true
	s.FromX, s.FromY, s.FromZ, s.ToX, s.ToY, s.ToZ = -2, -2, -2, 2, 2, 2
	assert.NoError(t, m.SetSettings(s))
	assert.Error(t, m.Start())

	// with areas file
	assert.NoError(t, os.WriteFile(path.Join(a.WorldDir, "areas.dat"), []byte("[]"), 0644))
	assert.NoError(t, m.Start())
	assert.Error(t, m.Start(), "already running")
	assert.Error(t, m.SetSettings(s), "settings locked while running")

	deadline := time.Now().Add(10 * time.Second)
	for {
		st, err = m.Status()
		assert.NoError(t, err)
		if st.Status != mapcleaner.StatusRunning || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	assert.Equal(t, mapcleaner.StatusDone, st.Status)
	assert.Equal(t, int64(125), st.Processed)
	assert.True(t, st.DryRun)
	assert.Error(t, m.Pause(), "not running")

	// pause/resume with a delay
	assert.NoError(t, m.Reset())
	s.DelayMs = 20
	assert.NoError(t, m.SetSettings(s))
	assert.NoError(t, m.Start())
	time.Sleep(50 * time.Millisecond)
	assert.NoError(t, m.Pause())

	st, err = m.Status()
	assert.NoError(t, err)
	assert.Equal(t, mapcleaner.StatusPaused, st.Status)
	assert.True(t, st.Processed > 0 && st.Processed < 125)
	processed := st.Processed

	// the state survives a new manager instance (mtui restart)
	m2 := app.NewMapCleaner(a)
	st, err = m2.Status()
	assert.NoError(t, err)
	assert.Equal(t, mapcleaner.StatusPaused, st.Status)
	assert.Equal(t, processed, st.Processed)

	// changing the range resets the progress
	s.ToX = 3
	assert.NoError(t, m.SetSettings(s))
	st, err = m.Status()
	assert.NoError(t, err)
	assert.Equal(t, mapcleaner.StatusIdle, st.Status)
	assert.Equal(t, int64(0), st.Processed)
}
