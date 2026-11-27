package mapcleaner_test

import (
	"context"
	"mtui/mapcleaner"
	"os"
	"path"
	"testing"

	"github.com/luanti-golang/areasparser"
	"github.com/luanti-golang/mtdb"
	"github.com/luanti-golang/mtdb/block"
	"github.com/stretchr/testify/assert"
)

func TestCoords(t *testing.T) {
	x, y, z := mapcleaner.MapblockPosFromNode(-15, 15, 16)
	assert.Equal(t, []int{-1, 0, 1}, []int{x, y, z})
	x, y, z = mapcleaner.MapblockPosFromNode(-1565, 4, -386)
	assert.Equal(t, []int{-98, 0, -25}, []int{x, y, z})

	assert.Equal(t, mapcleaner.ChunkPos{X: -1, Y: 0, Z: 1}, mapcleaner.ChunkPosFromMapblock(-1, 4, 5))
	assert.Equal(t, mapcleaner.ChunkPos{X: -1, Y: -1, Z: 0}, mapcleaner.ChunkPosFromNode(-80, -1, 79))
	assert.Equal(t, mapcleaner.ChunkPos{X: 1, Y: 0, Z: 0}, mapcleaner.ChunkPosFromNode(80, 0, 0))

	x1, y1, z1, x2, y2, z2 := mapcleaner.ChunkPos{X: -1, Y: 0, Z: 2}.MapblockBounds()
	assert.Equal(t, []int{-5, 0, 10, -1, 4, 14}, []int{x1, y1, z1, x2, y2, z2})
}

func TestSettingsValidate(t *testing.T) {
	s := mapcleaner.DefaultSettings()
	assert.NoError(t, s.Validate())

	s.UseAreas = false
	assert.Error(t, s.Validate(), "nothing protected")

	s.ProtectedNodes = []string{" # comment", "", "default:chest ", "default:chest"}
	s.Normalize()
	assert.Equal(t, []string{"default:chest"}, s.ProtectedNodes)
	assert.NoError(t, s.Validate())

	s.FromX = 41
	assert.Error(t, s.Validate(), "from > to")
	s.FromX = -500
	assert.Error(t, s.Validate(), "out of range")
}

func createBlockRepo(t *testing.T) block.BlockRepository {
	dir := t.TempDir()
	assert.NoError(t, os.WriteFile(path.Join(dir, "world.mt"), []byte("backend = sqlite3\n"), 0644))
	repo, err := mtdb.NewBlockDB(dir)
	assert.NoError(t, err)
	assert.NotNil(t, repo)
	return repo
}

// fills all mapblocks of the chunk with the given data
func fillChunk(t *testing.T, repo block.BlockRepository, pos mapcleaner.ChunkPos, data []byte) {
	x1, y1, z1, x2, y2, z2 := pos.MapblockBounds()
	for x := x1; x <= x2; x++ {
		for y := y1; y <= y2; y++ {
			for z := z1; z <= z2; z++ {
				assert.NoError(t, repo.Update(&block.Block{PosX: x, PosY: y, PosZ: z, Data: data}))
			}
		}
	}
}

func chunkExists(t *testing.T, repo block.BlockRepository, pos mapcleaner.ChunkPos) bool {
	x1, y1, z1, _, _, _ := pos.MapblockBounds()
	b, err := repo.GetByPos(x1, y1, z1)
	assert.NoError(t, err)
	return b != nil
}

var (
	chunkProtected = mapcleaner.ChunkPos{X: 0, Y: 0, Z: 0} // contains "protector:protect"
	chunkNeighbor  = mapcleaner.ChunkPos{X: 1, Y: 0, Z: 0} // next to the protected chunk
	chunkFar       = mapcleaner.ChunkPos{X: 3, Y: 0, Z: 0} // unprotected
	chunkArea      = mapcleaner.ChunkPos{X: 3, Y: 0, Z: 3} // inside an area
)

func setupWorld(t *testing.T) (block.BlockRepository, *mapcleaner.Settings, []*areasparser.Area) {
	repo := createBlockRepo(t)

	plain, err := os.ReadFile("testdata/zstd-block.bin")
	assert.NoError(t, err)
	protected, err := os.ReadFile("testdata/0.0.0")
	assert.NoError(t, err)

	fillChunk(t, repo, chunkProtected, plain)
	// a single protected mapblock in the chunk is enough
	assert.NoError(t, repo.Update(&block.Block{PosX: 2, PosY: 2, PosZ: 2, Data: protected}))
	fillChunk(t, repo, chunkNeighbor, plain)
	fillChunk(t, repo, chunkFar, plain)
	fillChunk(t, repo, chunkArea, plain)

	// area inside chunk 3,0,3 (nodes 240..319)
	areas := []*areasparser.Area{{
		Name:  "test",
		Owner: "singleplayer",
		Pos1:  &areasparser.GenericPos{X: 300, Y: 10, Z: 300},
		Pos2:  &areasparser.GenericPos{X: 250, Y: 0, Z: 250},
	}}

	s := &mapcleaner.Settings{
		ProtectedNodes: []string{"protector:protect"},
		UseAreas:       true,
		NeighborRange:  1,
		FromX:          -1, FromY: 0, FromZ: 0,
		ToX: 4, ToY: 0, ToZ: 4,
	}
	assert.NoError(t, s.Validate())
	return repo, s, areas
}

func noProgress(s mapcleaner.State, persist bool) error { return nil }

func TestRun(t *testing.T) {
	repo, s, areas := setupWorld(t)

	state := mapcleaner.NewState(s)
	assert.NoError(t, mapcleaner.New(repo, s, areas).Run(context.Background(), state, noProgress))

	assert.Equal(t, int64(30), state.Processed)
	assert.Equal(t, int64(1), state.Removed)
	assert.Equal(t, int64(3), state.Retained)

	assert.True(t, chunkExists(t, repo, chunkProtected))
	assert.True(t, chunkExists(t, repo, chunkNeighbor))
	assert.False(t, chunkExists(t, repo, chunkFar))
	assert.True(t, chunkExists(t, repo, chunkArea))

	count, err := repo.Count()
	assert.NoError(t, err)
	assert.Equal(t, int64(3*125), count)
}

func TestRunDryRun(t *testing.T) {
	repo, s, areas := setupWorld(t)
	s.DryRun = true

	state := mapcleaner.NewState(s)
	assert.NoError(t, mapcleaner.New(repo, s, areas).Run(context.Background(), state, noProgress))
	assert.Equal(t, int64(1), state.Removed)
	assert.True(t, chunkExists(t, repo, chunkFar))
}

func TestRunWithoutAreas(t *testing.T) {
	repo, s, _ := setupWorld(t)
	s.UseAreas = false

	state := mapcleaner.NewState(s)
	assert.NoError(t, mapcleaner.New(repo, s, nil).Run(context.Background(), state, noProgress))
	assert.Equal(t, int64(2), state.Removed)
	assert.False(t, chunkExists(t, repo, chunkArea))
}

func TestRunPauseResume(t *testing.T) {
	repo, s, areas := setupWorld(t)

	state := mapcleaner.NewState(s)
	ctx, cancel := context.WithCancel(context.Background())
	err := mapcleaner.New(repo, s, areas).Run(ctx, state, func(st mapcleaner.State, persist bool) error {
		if st.Processed == 10 {
			cancel()
		}
		return nil
	})
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, int64(10), state.Processed)

	// resume with a fresh cleaner (empty caches)
	persisted := 0
	assert.NoError(t, mapcleaner.New(repo, s, areas).Run(context.Background(), state, func(st mapcleaner.State, persist bool) error {
		if persist {
			persisted++
		}
		return nil
	}))
	assert.Equal(t, int64(30), state.Processed)
	assert.Equal(t, int64(1), state.Removed)
	assert.Equal(t, int64(3), state.Retained)
	assert.True(t, persisted > 0)
	assert.False(t, chunkExists(t, repo, chunkFar))
}
