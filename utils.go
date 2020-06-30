package main

func getNextID(id uint16) uint16 {
	nextID := uint16(0)
	if id < 60000 {
		nextID = id + 1
	}
	return nextID
}
