package internal

type Torrent struct {
	torrPath string
	destPath string

	localData       bencodedData
	networkDataUDP  []trackerUDP
	networkDataHTTP []trackerHTTP
}

func NewTorrent() *Torrent {
	return &Torrent{}
}
