package internal

import (
	"errors"
	"math/rand/v2"
	"net"
	"sync"

	"gitlab.com/LUwUcifer/gotorrent/logger"
	"go.uber.org/zap"
)

// TODO: Move the following files outside internal, in a separate package.
// 1. torrentPlayerS.go

const (
	playerUninitialized byte = iota
	playerPaused
	playerStarted
	playerStopped // The torrent player should not be used once stopped
)

type TorrentPlayer struct {
	torrent *Torrent

	state byte

	peerID        peerID
	key           uint32 // persistent across sessions?
	listeningPort uint16
	listener      *net.TCPListener
}

// NewTorrentPlayer returns a new uninitialized torrent player
func NewTorrentPlayer() *TorrentPlayer {
	return &TorrentPlayer{}
}

// AddTorrent adds a torrent. This function initializes the torrent player.
func (torrentPlayer *TorrentPlayer) AddTorrent(torrentSrcPath, destPath string) error {
	torrent := NewTorrent()
	err := torrent.ExtractTorrentInfo(torrentSrcPath, destPath)
	if err != nil {
		return err
	}
	return torrentPlayer.addTorrent(torrent)
}

// StartTorrent starts the associated torrent.
// Restarts a paused torrent player.
func (torrentPlayer *TorrentPlayer) StartTorrent() error {
	return torrentPlayer.startTorrent()

}

// PauseTorrent pauses the torrent player temporarily.
// Any leftover progress not saved to disk is saved for when the torrent is resumed.
func (torrentPlayer *TorrentPlayer) PauseTorrent() error {
	return torrentPlayer.pauseTorrent()

}

// StopTorrent stops the torrent player and frees all allocated resources.
// Any leftover progress not saved to disk is discarded.
func (torrentPlayer *TorrentPlayer) StopTorrent() error {
	return torrentPlayer.startTorrent()
}

func (torrentPlayer *TorrentPlayer) addTorrent(torrent *Torrent) error {
	var err error
	torrentPlayer.torrent = torrent
	torrentPlayer.peerID = peerIDGenerate()
	torrentPlayer.key = rand.Uint32()
	torrentPlayer.listener, torrentPlayer.listeningPort, err = initListener()
	if err != nil {
		return err
	}
	logger.Logger.Info("Initialized TCP Listener", zap.Uint16("port", torrentPlayer.listeningPort),
		zap.String("peerID", string(torrentPlayer.peerID[:])))
	return nil
}

func (torrentPlayer *TorrentPlayer) startTorrent() error {
	peerAdds, err := torrentPlayer.getPeersFromTrackers(40)
	if err != nil {
		return err
	}
	logger.Logger.Info("Acquired peer addresses", zap.Strings("peers", peerAdds))

	return nil
}

func (torrentPlayer *TorrentPlayer) pauseTorrent() error {
	return nil
}

func (torrentPlayer *TorrentPlayer) stopTorrent() error {
	return nil
}

func (torrentPlayer *TorrentPlayer) getPeersFromTrackers(numwant int) ([]string, error) {
	var tPeerAdds []string
	var err error
	for j, tier := range torrentPlayer.torrent.trackers {
		for i, iTracker := range tier {
			switch iTracker.getProtocol() {
			case pUDP:
				udpTracker := iTracker.(*trackerUDP)
				tPeerAdds, err = udpTracker.announce(
					torrentPlayer.torrent.localData.infoHash,
					torrentPlayer.peerID,
					udpTracker.trackerData.torrentSizeData,
					torrentPlayer.key,
					udpEventNone,
					uint32(numwant),
					torrentPlayer.listeningPort,
				)
				break

			case pHTTP:
			default:
				logger.Logger.Warn("Unknown tracker protocol", zap.Int("tier", j))
				continue
			}

			if err != nil {
				return nil, err
			} else if len(tPeerAdds) == 0 {
				continue
			}

			tier[0], tier[i] = tier[i], tier[0]
			return tPeerAdds, nil
		}
	}
	return nil, trackersUnresponsiveError
}

var (
	trackersUnresponsiveError = errors.New("no tracker responsive")
)
