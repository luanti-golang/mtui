package web

import (
	"encoding/json"
	"fmt"
	"mtui/mapcleaner"
	"mtui/types"
	"net/http"
)

func (a *Api) GetMapcleanerSettings(w http.ResponseWriter, r *http.Request, c *types.Claims) {
	s, err := a.app.MapCleaner.GetSettings()
	Send(w, s, err)
}

func (a *Api) SetMapcleanerSettings(w http.ResponseWriter, r *http.Request, c *types.Claims) {
	s := &mapcleaner.Settings{}
	err := json.NewDecoder(r.Body).Decode(s)
	if err != nil {
		SendError(w, http.StatusBadRequest, err)
		return
	}

	err = a.app.MapCleaner.SetSettings(s)
	if err != nil {
		SendError(w, http.StatusBadRequest, err)
		return
	}

	a.app.CreateUILogEntry(&types.Log{
		Username: c.Username,
		Event:    "mapcleaner",
		Message: fmt.Sprintf("User '%s' changed the map cleanup settings (chunks %d,%d,%d to %d,%d,%d, %d protected nodes, areas: %v, dry-run: %v)",
			c.Username, s.FromX, s.FromY, s.FromZ, s.ToX, s.ToY, s.ToZ, len(s.ProtectedNodes), s.UseAreas, s.DryRun),
	}, r)

	SendJson(w, s)
}

func (a *Api) GetMapcleanerStatus(w http.ResponseWriter, r *http.Request, c *types.Claims) {
	s, err := a.app.MapCleaner.Status()
	Send(w, s, err)
}

func (a *Api) mapcleanerAction(w http.ResponseWriter, r *http.Request, c *types.Claims, fn func() error, action string) {
	err := fn()
	if err != nil {
		SendError(w, http.StatusBadRequest, err)
		return
	}

	a.app.CreateUILogEntry(&types.Log{
		Username: c.Username,
		Event:    "mapcleaner",
		Message:  fmt.Sprintf("User '%s' %s the map cleanup", c.Username, action),
	}, r)

	s, err := a.app.MapCleaner.Status()
	Send(w, s, err)
}

func (a *Api) StartMapcleaner(w http.ResponseWriter, r *http.Request, c *types.Claims) {
	a.mapcleanerAction(w, r, c, a.app.MapCleaner.Start, "started")
}

func (a *Api) PauseMapcleaner(w http.ResponseWriter, r *http.Request, c *types.Claims) {
	a.mapcleanerAction(w, r, c, a.app.MapCleaner.Pause, "paused")
}

func (a *Api) ResetMapcleaner(w http.ResponseWriter, r *http.Request, c *types.Claims) {
	a.mapcleanerAction(w, r, c, a.app.MapCleaner.Reset, "reset")
}
