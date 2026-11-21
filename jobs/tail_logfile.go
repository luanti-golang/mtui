package jobs

import (
	"mtui/app"
	"mtui/types"
	"strings"
	"time"

	"github.com/hpcloud/tail"
	"github.com/sirupsen/logrus"
)

func tailLogfile(a *app.App) {
	for {
		t, err := tail.TailFile(a.Config.TailEngineLogfile, tail.Config{Follow: true})
		if err != nil {
			logrus.WithError(err).Error("TailFile error")
		} else {
			// logfile opened
			if !processLines(a, t.Lines) {
				return
			}
			t.Cleanup()
		}

		// re-schedule
		time.Sleep(time.Second * 10)
	}
}

const errorFlushDelay = time.Millisecond * 500

// engine log line prefix, example: "2026-11-18 23:57:49: ERROR[Main]: ..."
const engineLogTimestampLayout = "2006-01-02 15:04:05"

// parseEngineLogTimestamp splits the timestamp prefix off a single line
// returns the unix-millis timestamp (0 if none found) and the remaining text
func parseEngineLogTimestamp(line string) (int64, string) {
	l := len(engineLogTimestampLayout)
	if len(line) < l+1 || line[l] != ':' {
		return 0, line
	}
	t, err := time.ParseInLocation(engineLogTimestampLayout, line[:l], time.Local)
	if err != nil {
		return 0, line
	}
	return t.UnixMilli(), strings.TrimPrefix(line[l+1:], " ")
}

// stripEngineLogTimestamps removes the timestamp prefix from every line of the message
// returns the timestamp of the first line (0 if none found) and the stripped message
func stripEngineLogTimestamps(message string) (int64, string) {
	lines := strings.Split(message, "\n")
	var ts int64
	for i, line := range lines {
		lineTs, text := parseEngineLogTimestamp(line)
		if i == 0 {
			ts = lineTs
		}
		lines[i] = text
	}
	return ts, strings.Join(lines, "\n")
}

func insertEngineLog(a *app.App, event, message string) error {
	ts, message := stripEngineLogTimestamps(message)
	return a.Repos.LogRepository.Insert(&types.Log{
		// falls back to the current time in the repository if 0
		Timestamp: ts,
		Category:  types.CategoryService,
		Event:     event,
		Message:   message,
	})
}

// processLines consumes log lines, merging consecutive mod-error lines into a single multi-line log entry.
// returns false on a fatal insert error
func processLines(a *app.App, lines chan *tail.Line) bool {
	var pending []string

	flush := func() bool {
		if len(pending) == 0 {
			return true
		}
		err := insertEngineLog(a, "engine-mod-errors", strings.Join(pending, "\n"))
		pending = nil
		if err != nil {
			logrus.WithError(err).Error("could not insert log")
			return false
		}
		return true
	}

	for {
		var timeout <-chan time.Time
		if len(pending) > 0 {
			timeout = time.After(errorFlushDelay)
		}

		select {
		case <-timeout:
			if !flush() {
				return false
			}
		case line, ok := <-lines:
			if !ok {
				return flush()
			}
			if line.Err != nil {
				logrus.WithError(line.Err).Error("TailFile.lines error")
				flush()
				return true
			}

			if strings.Contains(line.Text, "ERROR[Main]") {
				pending = append(pending, line.Text)
				continue
			}

			// sequence ended
			if !flush() {
				return false
			}
			if err := insertEngineLog(a, "engine", line.Text); err != nil {
				logrus.WithError(err).Error("could not insert log")
				return false
			}
		}
	}
}
