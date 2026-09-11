package internal

type Torrent struct {
	torrPath string
	destPath string
	torrName string

	localData bencodedData

	// 2-Dimensional tracker list for tiered announce-list
	trackers [][]tracker
}

func NewTorrent() *Torrent {
	return &Torrent{}
}
