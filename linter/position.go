package linter

import "sort"

// positions converts byte offsets into 1-based line/column pairs.
type positions struct {
	// lineStarts holds the byte offset of the first byte of each line.
	lineStarts []int
}

func newPositions(src []byte) *positions {
	starts := []int{0}
	for i, b := range src {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return &positions{lineStarts: starts}
}

func (p *positions) lineCol(offset int) (line, col int) {
	i := sort.Search(len(p.lineStarts), func(i int) bool {
		return p.lineStarts[i] > offset
	}) - 1
	return i + 1, offset - p.lineStarts[i] + 1
}
