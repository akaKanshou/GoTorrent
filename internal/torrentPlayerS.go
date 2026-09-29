package internal

import (
	"errors"
	"math/rand/v2"
	"net"
	"slices"
	"sync"
	"time"

	"gitlab.com/akaKanshou/gotorrent/logger"
	"go.loafoe.dev/bitfield/v2"
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

	peers []*peer // List of currently connected and active peers

	Wg        sync.WaitGroup
	eventChan chan event

	fileManager *fileManager
}

// NewTorrentPlayer returns a new uninitialized torrent player
func NewTorrentPlayer() *TorrentPlayer {
	return &TorrentPlayer{
		state:     playerUninitialized,
		Wg:        sync.WaitGroup{},
		eventChan: make(chan event, 256),
	}
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

	torrentPlayer.fileManager = newFileManager(torrent.destPath, int64(torrent.localData.infoDict.pieceLength), torrent.localData.infoDict.pieces)
	err = torrentPlayer.fileManager.initFiles(torrent.localData.infoDict)
	return nil
}

func (torrentPlayer *TorrentPlayer) startTorrent() error {

	torrentPlayer.state = playerStarted
	go torrentPlayer.eventLoop()
	torrentPlayer.Wg.Add(1)

	peerAdds, err := torrentPlayer.getPeersFromTrackers(40)
	if err != nil {
		return err
	}
	logger.Logger.Info("Acquired peer addresses", zap.Strings("peers", peerAdds))

	for _, peerAddr := range peerAdds {
		go torrentPlayer.initiateHandshake(peerAddr)
		torrentPlayer.Wg.Add(1)
	}

	torrentPlayer.Wg.Wait()

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
				if time.Since(udpTracker.lastAnnounce) < udpTracker.trackerData.interval {
					logger.Logger.Info("Skipping UDP Announce; Announce too frequent", zap.String("tracker", udpTracker.trackerData.metaData.announceUrl))
					break
				}

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
				logger.Logger.Error("Failed to announce peer", zap.Int("tier", j), zap.Error(err))
				continue
			} else if len(tPeerAdds) == 0 {
				continue
			}

			tier[0], tier[i] = tier[i], tier[0]
			return tPeerAdds, nil
		}
	}
	return nil, trackersUnresponsiveError
}

// Goroutine to accept peers through torrentPlayer.peerChan
// 1. Any peer with which a handshake has been successful is sent through torrentPlayer.peerChan to get added.
// This includes both peers with whom a handshake was initiated by client (which the client got from the trackers)
// and the ones who initiated handshakes (peers that got the client address from their own announces to the trackers)
// 2. Any peer that is to be destroyed is also sent through torrentPlayer.peerChan with the destroyed field set to true.
func (torrentPlayer *TorrentPlayer) acceptPeers(nPeer *peer) {
	torrentPlayer.peers = append(torrentPlayer.peers, nPeer)
	logger.Logger.Debug("Accepted peer", zap.String("peer", string(nPeer.peerID[:])),
		zap.String("peerIPAddr", nPeer.conn.LocalAddr().String()))

	go torrentPlayer.listenToPeer(nPeer)
	go torrentPlayer.keepAlive(nPeer)
	torrentPlayer.Wg.Add(2)
}

func (torrentPlayer *TorrentPlayer) destroyPeer(nPeer *peer) {
	index := slices.IndexFunc(torrentPlayer.peers, func(rPeer *peer) bool {
		return slices.Compare(nPeer.peerID[:], rPeer.peerID[:]) == 0
	})
	if index == -1 {
		logger.Logger.Warn("Tried to destroy non-existent peer", zap.String("peer", string(nPeer.peerID[:])))
		return
	} else {
		// Have not used slices.Delete in favor of this homemade deletion for efficient deletion
		lenPeers := len(torrentPlayer.peers)
		torrentPlayer.peers[index].conn.Close()
		torrentPlayer.peers[index] = torrentPlayer.peers[lenPeers-1]
		torrentPlayer.peers = torrentPlayer.peers[:lenPeers-1]
	}
	logger.Logger.Debug("Destroyed peer", zap.String("peer", string(nPeer.peerID[:])))
}

var (
	trackersUnresponsiveError = errors.New("no tracker responsive")
)

func (torrentPlayer *TorrentPlayer) eventLoop() {
	defer torrentPlayer.Wg.Done()

	var err error
	for {
		event, ok := <-torrentPlayer.eventChan
		if !ok {
			return
		}

		switch event.eventType() {
		case tpEventNewPeer:
			newPeerEvent := event.(*newPeerEvent)
			torrentPlayer.acceptPeers(newPeerEvent.newPeer)
			continue

		case tpEventDestroyPeer:
			destroyPeerEvent := event.(*destroyPeerEvent)
			torrentPlayer.destroyPeer(destroyPeerEvent.peer)
			continue

		case tpEventIncomingMessage:
			incomingMessageEvent := event.(*incomingMessageEvent)
			err = torrentPlayer.handleMessage(incomingMessageEvent.peer, incomingMessageEvent.message)
			if err != nil {
				logger.Logger.Error("Failed to handle incoming message", zap.Error(err))
				// TODO: Add check if error was peer-side or client-side
				torrentPlayer.destroyPeer(incomingMessageEvent.peer)
			}
			continue

		case tpPeerAvailableForDownloadEvent:
			peerAvailableForDownloadEvent := event.(*peerAvailableForDownloadEvent)
			index := torrentPlayer.fileManager.getPieceToDownload(peerAvailableForDownloadEvent.peer.piecesAvailable)
			if index == -1 {
				logger.Logger.Debug("Peer does not have any relevant pieces", zap.String("peer", string(peerAvailableForDownloadEvent.peer.peerID[:])))
				peerAvailableForDownloadEvent.peer.amInterested = false
				go torrentPlayer.sendMessage(peerAvailableForDownloadEvent.peer, &notInterestedMessage{})
			} else {
				pieceLen := torrentPlayer.fileManager.getPieceLength(index)
				torrentPlayer.fileManager.piecesInProgress[index] = peerAvailableForDownloadEvent.peer
				peerAvailableForDownloadEvent.peer.initBuffers(index, pieceLen)
				torrentPlayer.sendMessage(peerAvailableForDownloadEvent.peer, newPipedRequestsMessage(index, pieceLen))
			}
			continue

		case tpVerifyPieceEvent:
			verifyPieceEvent := event.(*verifyPieceEvent)
			delete(torrentPlayer.fileManager.piecesInProgress, verifyPieceEvent.index)
			// TODO: Edit torrent download size

			shaVerified := torrentPlayer.fileManager.verifyPiece(verifyPieceEvent.peer.pieceBuffer[:verifyPieceEvent.peer.pieceLength], verifyPieceEvent.peer.pieceDownloading)
			if !shaVerified {
				logger.Logger.Error("Piece SHA hash does not match")
				verifyPieceEvent.peer.piecesAvailable.ClearBit(uint64(verifyPieceEvent.peer.pieceDownloading))
				torrentPlayer.eventChan <- &peerAvailableForDownloadEvent{peer: verifyPieceEvent.peer}
				continue
			}
			err := torrentPlayer.fileManager.writePiece(verifyPieceEvent.peer.pieceDownloading, verifyPieceEvent.peer.pieceLength, verifyPieceEvent.peer.pieceBuffer)
			if err != nil {
				logger.Logger.Error("Failed to write piece", zap.Error(err))
			} else if torrentPlayer.fileManager.piecesDownloaded == len(torrentPlayer.fileManager.pieces) {
				logger.Logger.Info("Torrent Completed!!!")
				return
			}
			torrentPlayer.eventChan <- &peerAvailableForDownloadEvent{peer: verifyPieceEvent.peer}
			continue
		}
	}
}

// keepAlive tries sending keep alive at regular intervals of inactivity.
// Inactivity is reset at every incoming and outgoing message from the peer
// At 2 minutes of inactivity, peer is dropped.
func (torrentPlayer *TorrentPlayer) keepAlive(rPeer *peer) {
	defer torrentPlayer.Wg.Done()

	timer := time.NewTimer(time.Minute * 2)
	defer timer.Stop()

	for {
		select {
		case _, ok := <-rPeer.peerActivity:
			if !ok || rPeer.destroyed.Load() {
				return
			}
			timer.Reset(time.Minute * 2)
			continue

		case <-time.After(45 * time.Second):
			go torrentPlayer.sendMessage(rPeer, &keepAliveMessage{})
			continue

		case <-timer.C:
			logger.Logger.Debug("Peer timed out", zap.String("peer", string(rPeer.peerID[:])))
			rPeer.destroyed.Store(true)
			torrentPlayer.eventChan <- &destroyPeerEvent{rPeer}
			return
		}
	}
}
