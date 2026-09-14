package internal

import "time"

//Min Announce for UDP
//TODO: Check port properties, whether to implement in tracker get struct

type peerID [20]byte
type peer struct {
	host   string
	port   uint16
	peerID peerID
}

type metaData struct {
	announceUrl string
	infoHash    hashBytes
}

type torrentSizeData struct {
	Downloaded uint64
	Uploaded   uint64
	Left       uint64
}

type trackerData struct {
	peers           []peer
	metaData        metaData
	torrentSizeData torrentSizeData
	interval        time.Duration
	seeders         uint32
	leechers        uint32
}

type trackerUDP struct {
	trackerData  trackerData
	retryCounter uint32
	connectionID uint64
	lastConnect  time.Time
	lastAnnounce time.Time
}

// TODO: Implement HTTP error handling
type trackerHTTP struct {
	trackerData trackerData
	minInterval time.Duration
	trackerID   string
}

//[]peers
//Interval, Min/Next Announce
//Seeders peers
//UDP
//Transaction ID Send, Echoes by UDP
