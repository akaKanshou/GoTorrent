package internal

import (
	"net"
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
	return nil
}

func (torrentPlayer *TorrentPlayer) startTorrent() error {
	return nil
}

func (torrentPlayer *TorrentPlayer) pauseTorrent() error {
	return nil
}

func (torrentPlayer *TorrentPlayer) stopTorrent() error {
	return nil
}
