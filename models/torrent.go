package models

type Torrent struct {
	localData   bencodedData
	networkData trackerData
}

func NewTorrent() *Torrent {
	return &Torrent{}
}
