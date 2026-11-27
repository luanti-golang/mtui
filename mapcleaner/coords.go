package mapcleaner

// ChunkSize is the edge length of a map chunk in mapblocks
const ChunkSize = 5

// MapblockSize is the edge length of a mapblock in nodes
const MapblockSize = 16

type ChunkPos struct {
	X int
	Y int
	Z int
}

// floorDiv divides and rounds towards negative infinity
func floorDiv(n, q int) int {
	d := n / q
	if (n%q != 0) && ((n < 0) != (q < 0)) {
		d--
	}
	return d
}

func MapblockPosFromNode(x, y, z int) (int, int, int) {
	return floorDiv(x, MapblockSize), floorDiv(y, MapblockSize), floorDiv(z, MapblockSize)
}

func ChunkPosFromMapblock(x, y, z int) ChunkPos {
	return ChunkPos{floorDiv(x, ChunkSize), floorDiv(y, ChunkSize), floorDiv(z, ChunkSize)}
}

func ChunkPosFromNode(x, y, z int) ChunkPos {
	return ChunkPosFromMapblock(MapblockPosFromNode(x, y, z))
}

// MapblockBounds returns the inclusive mapblock range of the chunk
func (c ChunkPos) MapblockBounds() (x1, y1, z1, x2, y2, z2 int) {
	x1 = c.X * ChunkSize
	y1 = c.Y * ChunkSize
	z1 = c.Z * ChunkSize
	x2 = x1 + ChunkSize - 1
	y2 = y1 + ChunkSize - 1
	z2 = z1 + ChunkSize - 1
	return
}
