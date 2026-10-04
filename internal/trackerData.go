package internal

import (
	"net"
	"sync/atomic"
	"time"

	"go.loafoe.dev/bitfield/v2"
)

//Min Announce for UDP
//TODO: Check port properties, whether to implement in tracker get struct

type peerID [20]byte
type peer struct {
	conn   net.Conn
	peerID peerID

	options uint64 // BitTorrent extensions supported by the peer

	amChoking      bool
	amInterested   bool
	peerChoking    bool
	peerInterested bool

	// Pieces that the piece has available for sharing, as reported by the peer
	piecesAvailable *bitfield.BitField

	// Last time peer sent or received a message. If time since lastMessage > 2 minutes, drop peer
	peerActivity    chan any
	sendMessageChan chan peerwireMessage

	// If this is an invalid or a dummy peer used to indicate a removal
	// If destroyed is set to true, do not use peer
	destroyed atomic.Bool

	// Buffers and information about the piece being downloaded from the peer
	pieceDownloading int64
	pieceBuffer      []byte
	bytesRead        *bitfield.BitField
	pieceLength      int64
	bytesDownloaded  int64

	downloadRate        atomic.Int64
	uploadRate          int64
	lastBlockUploadTime time.Time
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
	peers    []peer
	metaData metaData
	interval time.Duration
	seeders  uint32
	leechers uint32
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
