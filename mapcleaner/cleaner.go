// Package mapcleaner removes map chunks that contain no protected nodes and
// are not covered by protected areas (ported from github.com/luanti-golang/mapcleaner)
package mapcleaner

import (
	"context"
	"time"

	"github.com/luanti-golang/areasparser"
	"github.com/luanti-golang/mapparser"
	"github.com/luanti-golang/mtdb/block"
	"github.com/sirupsen/logrus"
)

// upper bound for the protection cache before it gets pruned on every z-stride
const maxCacheEntries = 1_000_000

type Cleaner struct {
	blocks         block.BlockRepository
	settings       *Settings
	protectedNodes map[string]bool
	protectedAreas map[ChunkPos]bool
	cache          map[ChunkPos]bool
}

// New creates a cleaner, areas may be nil if areas protection is disabled
func New(blocks block.BlockRepository, settings *Settings, areas []*areasparser.Area) *Cleaner {
	c := &Cleaner{
		blocks:         blocks,
		settings:       settings,
		protectedNodes: map[string]bool{},
		protectedAreas: map[ChunkPos]bool{},
		cache:          map[ChunkPos]bool{},
	}
	for _, name := range settings.ProtectedNodes {
		c.protectedNodes[name] = true
	}
	for _, area := range areas {
		c.addArea(area)
	}
	return c
}

func (c *Cleaner) addArea(area *areasparser.Area) {
	if area == nil || area.Pos1 == nil || area.Pos2 == nil {
		return
	}
	p1 := ChunkPosFromNode(min(area.Pos1.X, area.Pos2.X), min(area.Pos1.Y, area.Pos2.Y), min(area.Pos1.Z, area.Pos2.Z))
	p2 := ChunkPosFromNode(max(area.Pos1.X, area.Pos2.X), max(area.Pos1.Y, area.Pos2.Y), max(area.Pos1.Z, area.Pos2.Z))
	for x := p1.X; x <= p2.X; x++ {
		for y := p1.Y; y <= p2.Y; y++ {
			for z := p1.Z; z <= p2.Z; z++ {
				c.protectedAreas[ChunkPos{x, y, z}] = true
			}
		}
	}
}

// ProgressFunc is called after every processed chunk, persist is true on z-stride boundaries and at the end
type ProgressFunc func(s State, persist bool) error

// Run processes the chunks starting at the position in the state until the range is done
// or the context is cancelled. The state always points to the next unprocessed chunk.
func (c *Cleaner) Run(ctx context.Context, state *State, progress ProgressFunc) error {
	s := c.settings
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		if state.ChunkX > s.ToX {
			// next z-stride
			state.ChunkX = s.FromX
			state.ChunkZ++
			if len(c.cache) > maxCacheEntries {
				c.pruneCache(func(p ChunkPos) bool { return p.Z < state.ChunkZ-s.NeighborRange })
			}
			if err := progress(*state, true); err != nil {
				return err
			}
		}
		if state.ChunkZ > s.ToZ {
			// next y-layer
			state.ChunkX = s.FromX
			state.ChunkZ = s.FromZ
			state.ChunkY++
			c.pruneCache(func(p ChunkPos) bool { return p.Y < state.ChunkY-s.NeighborRange })
			logrus.WithFields(logrus.Fields{"chunk_y": state.ChunkY}).Info("mapcleaner: processing next y-layer")
		}
		if state.ChunkY > s.ToY {
			return progress(*state, true)
		}

		pos := ChunkPos{state.ChunkX, state.ChunkY, state.ChunkZ}
		emerged, err := c.isEmerged(pos)
		if err != nil {
			return err
		}
		if emerged {
			protected, err := c.isProtectedWithNeighbors(pos)
			if err != nil {
				return err
			}
			if protected {
				state.Retained++
			} else {
				if !s.DryRun {
					err = c.removeChunk(pos)
					if err != nil {
						return err
					}
				}
				state.Removed++
			}
		}
		state.Processed++
		state.ChunkX++

		if err := progress(*state, false); err != nil {
			return err
		}

		if s.DelayMs > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(s.DelayMs) * time.Millisecond):
			}
		}
	}
}

func (c *Cleaner) pruneCache(drop func(p ChunkPos) bool) {
	for p := range c.cache {
		if drop(p) {
			delete(c.cache, p)
		}
	}
}

// isEmerged checks the 8 corner mapblocks of the chunk for existence
func (c *Cleaner) isEmerged(pos ChunkPos) (bool, error) {
	x1, y1, z1, x2, y2, z2 := pos.MapblockBounds()
	for _, x := range []int{x1, x2} {
		for _, y := range []int{y1, y2} {
			for _, z := range []int{z1, z2} {
				b, err := c.blocks.GetByPos(x, y, z)
				if err != nil {
					return false, err
				}
				if b != nil {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

// isProtected checks if the chunk is inside a protected area or contains a protected node
func (c *Cleaner) isProtected(pos ChunkPos) (bool, error) {
	if c.protectedAreas[pos] {
		return true, nil
	}
	if p, ok := c.cache[pos]; ok {
		return p, nil
	}

	protected, err := c.containsProtectedNode(pos)
	if err != nil {
		return false, err
	}
	c.cache[pos] = protected
	return protected, nil
}

func (c *Cleaner) containsProtectedNode(pos ChunkPos) (bool, error) {
	if len(c.protectedNodes) == 0 {
		return false, nil
	}

	x1, y1, z1, x2, y2, z2 := pos.MapblockBounds()
	for x := x1; x <= x2; x++ {
		for y := y1; y <= y2; y++ {
			for z := z1; z <= z2; z++ {
				b, err := c.blocks.GetByPos(x, y, z)
				if err != nil {
					return false, err
				}
				if b == nil {
					continue
				}

				mb, err := mapparser.Parse(b.Data)
				if err != nil {
					// don't remove what can't be inspected
					logrus.WithError(err).WithFields(logrus.Fields{
						"x": x, "y": y, "z": z,
					}).Warn("mapcleaner: mapblock parse error, treating chunk as protected")
					return true, nil
				}

				for _, name := range mb.BlockMapping {
					if c.protectedNodes[name] {
						return true, nil
					}
				}
			}
		}
	}
	return false, nil
}

// isProtectedWithNeighbors checks the chunk and its surroundings within the neighbor range
func (c *Cleaner) isProtectedWithNeighbors(pos ChunkPos) (bool, error) {
	r := c.settings.NeighborRange
	for x := -r; x <= r; x++ {
		for y := -r; y <= r; y++ {
			for z := -r; z <= r; z++ {
				p, err := c.isProtected(ChunkPos{pos.X + x, pos.Y + y, pos.Z + z})
				if err != nil {
					return false, err
				}
				if p {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func (c *Cleaner) removeChunk(pos ChunkPos) error {
	x1, y1, z1, x2, y2, z2 := pos.MapblockBounds()
	for x := x1; x <= x2; x++ {
		for y := y1; y <= y2; y++ {
			for z := z1; z <= z2; z++ {
				err := c.blocks.Delete(x, y, z)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}
