package web_test

import (
	"bytes"
	"encoding/json"
	"mtui/mapcleaner"
	"mtui/types"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapcleanerApi(t *testing.T) {
	api, _ := CreateTestApi(t)
	claims := &types.Claims{Username: "singleplayer"}

	// defaults
	w := httptest.NewRecorder()
	api.GetMapcleanerSettings(w, httptest.NewRequest("GET", "http://", nil), claims)
	assert.Equal(t, 200, w.Code)
	s := &mapcleaner.Settings{}
	assert.NoError(t, json.NewDecoder(w.Body).Decode(s))
	assert.True(t, s.DryRun)

	// invalid settings
	s.UseAreas = false
	s.ProtectedNodes = []string{}
	buf, _ := json.Marshal(s)
	w = httptest.NewRecorder()
	api.SetMapcleanerSettings(w, httptest.NewRequest("POST", "http://", bytes.NewBuffer(buf)), claims)
	assert.Equal(t, 400, w.Code)

	// valid settings, normalized in the response
	s.ProtectedNodes = []string{" bones:bones", "", "bones:bones"}
	s.ToX = s.FromX
	buf, _ = json.Marshal(s)
	w = httptest.NewRecorder()
	api.SetMapcleanerSettings(w, httptest.NewRequest("POST", "http://", bytes.NewBuffer(buf)), claims)
	assert.Equal(t, 200, w.Code)
	assert.NoError(t, json.NewDecoder(w.Body).Decode(s))
	assert.Equal(t, []string{"bones:bones"}, s.ProtectedNodes)

	// status reflects the new range
	w = httptest.NewRecorder()
	api.GetMapcleanerStatus(w, httptest.NewRequest("GET", "http://", nil), claims)
	assert.Equal(t, 200, w.Code)
	status := map[string]any{}
	assert.NoError(t, json.NewDecoder(w.Body).Decode(&status))
	assert.Equal(t, "idle", status["status"])
	assert.Equal(t, float64(s.TotalChunks()), status["total_chunks"])

	// pausing a stopped run fails
	w = httptest.NewRecorder()
	api.PauseMapcleaner(w, httptest.NewRequest("POST", "http://", nil), claims)
	assert.Equal(t, 400, w.Code)
}
