package jobs

import (
	"mtui/app"

	"github.com/sirupsen/logrus"
)

// continues a map cleanup that was running when mtui was stopped
func mapcleanerResume(a *app.App) {
	err := a.MapCleaner.ResumeInterrupted()
	if err != nil {
		logrus.WithError(err).Error("mapcleaner: could not resume interrupted run")
	}
}
