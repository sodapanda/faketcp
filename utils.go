package main

import (
	"math"
)

func getNextID(id uint16) uint16 {
	return id + 1
}

func minAlignSize(length int, segs int) int {
	rst := math.Ceil(float64(length) / float64(segs))
	return int(rst) * segs
}
