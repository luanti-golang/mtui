package jobs

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseEngineLogTimestamp(t *testing.T) {
	expected := time.Date(2026, 11, 18, 23, 57, 49, 0, time.Local).UnixMilli()

	ts, text := parseEngineLogTimestamp(`2026-11-18 23:57:49: ERROR[Main]: Failed to overwrite "/root/.minetest/worlds/world/metatool.cfg": Resource busy`)
	assert.Equal(t, expected, ts)
	assert.Equal(t, `ERROR[Main]: Failed to overwrite "/root/.minetest/worlds/world/metatool.cfg": Resource busy`, text)

	for _, line := range []string{"", "ERROR[Main]: something", "2026-11-18 23:57:49", "2026-13-45 99:99:99: garbage"} {
		ts, text = parseEngineLogTimestamp(line)
		assert.Equal(t, int64(0), ts)
		assert.Equal(t, line, text)
	}
}

func TestStripEngineLogTimestamps(t *testing.T) {
	expected := time.Date(2026, 11, 18, 23, 57, 49, 0, time.Local).UnixMilli()

	ts, msg := stripEngineLogTimestamps("2026-11-18 23:57:49: ERROR[Main]: a\n2026-11-18 23:57:50: ERROR[Main]: b\nno prefix")
	assert.Equal(t, expected, ts)
	assert.Equal(t, "ERROR[Main]: a\nERROR[Main]: b\nno prefix", msg)

	ts, msg = stripEngineLogTimestamps("plain line")
	assert.Equal(t, int64(0), ts)
	assert.Equal(t, "plain line", msg)
}
