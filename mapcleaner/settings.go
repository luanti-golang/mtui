package mapcleaner

import (
	"errors"
	"fmt"
	"strings"
)

// Settings configures a cleanup run, ranges are inclusive chunk coordinates
type Settings struct {
	ProtectedNodes []string `json:"protected_nodes"`
	UseAreas       bool     `json:"use_areas"`
	NeighborRange  int      `json:"neighbor_range"`
	FromX          int      `json:"from_x"`
	FromY          int      `json:"from_y"`
	FromZ          int      `json:"from_z"`
	ToX            int      `json:"to_x"`
	ToY            int      `json:"to_y"`
	ToZ            int      `json:"to_z"`
	DelayMs        int      `json:"delay_ms"`
	DryRun         bool     `json:"dry_run"`
}

// world limits are +/- 31000 nodes -> +/- 388 chunks
const maxChunkCoord = 400

func DefaultSettings() *Settings {
	return &Settings{
		ProtectedNodes: []string{},
		UseAreas:       true,
		NeighborRange:  1,
		FromX:          -40,
		FromY:          -10,
		FromZ:          -40,
		ToX:            40,
		ToY:            10,
		ToZ:            40,
		DelayMs:        0,
		DryRun:         true,
	}
}

// Normalize trims and deduplicates the protected node list and drops comments
func (s *Settings) Normalize() {
	seen := map[string]bool{}
	nodes := []string{}
	for _, n := range s.ProtectedNodes {
		n = strings.TrimSpace(n)
		if n == "" || strings.HasPrefix(n, "#") || seen[n] {
			continue
		}
		seen[n] = true
		nodes = append(nodes, n)
	}
	s.ProtectedNodes = nodes
}

func (s *Settings) Validate() error {
	if len(s.ProtectedNodes) == 0 && !s.UseAreas {
		return errors.New("refusing to run without protected nodes or areas protection, everything would be removed")
	}
	if s.NeighborRange < 0 || s.NeighborRange > 5 {
		return fmt.Errorf("neighbor range must be between 0 and 5, got %d", s.NeighborRange)
	}
	if s.DelayMs < 0 {
		return fmt.Errorf("delay must not be negative, got %d", s.DelayMs)
	}
	for _, c := range []int{s.FromX, s.FromY, s.FromZ, s.ToX, s.ToY, s.ToZ} {
		if c < -maxChunkCoord || c > maxChunkCoord {
			return fmt.Errorf("chunk coordinate %d out of range (+/- %d)", c, maxChunkCoord)
		}
	}
	if s.FromX > s.ToX || s.FromY > s.ToY || s.FromZ > s.ToZ {
		return errors.New("the 'from' coordinates must be lower or equal to the 'to' coordinates")
	}
	return nil
}

// SameRange returns true if both settings cover the same chunk range
func (s *Settings) SameRange(o *Settings) bool {
	return s.FromX == o.FromX && s.FromY == o.FromY && s.FromZ == o.FromZ &&
		s.ToX == o.ToX && s.ToY == o.ToY && s.ToZ == o.ToZ
}

// TotalChunks returns the number of chunks in the configured range
func (s *Settings) TotalChunks() int64 {
	return int64(s.ToX-s.FromX+1) * int64(s.ToY-s.FromY+1) * int64(s.ToZ-s.FromZ+1)
}

type Status string

const (
	StatusIdle    Status = "idle"
	StatusRunning Status = "running"
	StatusPaused  Status = "paused"
	StatusDone    Status = "done"
	StatusError   Status = "error"
)

// State is the persisted progress of a run
type State struct {
	Status    Status `json:"status"`
	ChunkX    int    `json:"chunk_x"`
	ChunkY    int    `json:"chunk_y"`
	ChunkZ    int    `json:"chunk_z"`
	Processed int64  `json:"processed"`
	Removed   int64  `json:"removed"`
	Retained  int64  `json:"retained"`
	DryRun    bool   `json:"dry_run"`
	Error     string `json:"error"`
	StartedAt int64  `json:"started_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// NewState returns a fresh state positioned at the start of the range
func NewState(s *Settings) *State {
	return &State{
		Status: StatusIdle,
		ChunkX: s.FromX,
		ChunkY: s.FromY,
		ChunkZ: s.FromZ,
	}
}
