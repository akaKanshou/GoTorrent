package internal

import (
	"errors"
	"net"
	"time"

	"gitlab.com/LUwUcifer/gotorrent/logger"
	"go.uber.org/zap"
)

// initListener inits a *net.TCPListener, trying the range of ports 6881-6889
// Returns the listener and the port it is listening on.
func initListener() (*net.TCPListener, uint16, error) {
	for port := 6881; port <= 6889; port++ {
		listener, err := net.ListenTCP("tcp4", &net.TCPAddr{Port: port})
		if err == nil {
			return listener, uint16(port), err
		}
	}
	return nil, 0, listenerInitFailed
}

func (torrentPlayer *TorrentPlayer) initiateHandshake(peerAddr string) {
	defer torrentPlayer.Wg.Done()
	peerTCPAddr, err := net.ResolveTCPAddr("tcp", peerAddr)
	if err != nil {
		logger.Logger.Error("could not initiate handshake", zap.String("peer", peerAddr), zap.Error(err))
		return
	}
	logger.Logger.Debug("peer TCP address", zap.String("peer", peerTCPAddr.String()), zap.String("network", peerTCPAddr.Network()))

	peerConn, err := net.DialTimeout("tcp", peerAddr, time.Minute)
	if err != nil {
		logger.Logger.Error("could not initiate handshake", zap.String("peer", peerAddr), zap.Error(err))
		return
	}
	defer peerConn.SetDeadline(time.Time{})

	req := handshakeMessage{
		peerID:   torrentPlayer.peerID,
		infoHash: torrentPlayer.torrent.localData.infoHash,
	}

	peerConn.SetDeadline(time.Now().Add(time.Minute))
	n, err := peerConn.Write(req.marshalMessage())
	if err != nil {
		logger.Logger.Error("could not send handshake", zap.String("peer", peerAddr), zap.Error(err))
		return
	} else if n != len(req.marshalMessage()) {
		logger.Logger.Error("could not send handshake", zap.String("peer", peerAddr), zap.Int("bytes written", n))
		return
	}

	peerConn.SetDeadline(time.Now().Add(time.Minute))
	responseBuf := make([]byte, handshakeLength)
	n, err = peerConn.Read(responseBuf)
	if err != nil {
		logger.Logger.Error("could not read handshake", zap.String("peer", peerAddr), zap.Error(err))
		return
	} else if n != handshakeLength {
		logger.Logger.Error("could not read handshake", zap.String("peer", peerAddr), zap.Int("bytes read", n))
		return
	}

	res := handshakeMessage{}
	err = res.unmarshalMessage(responseBuf)
	if err != nil {
		logger.Logger.Error("invalid handshake response", zap.String("peer", peerAddr), zap.Error(err))
	}
	logger.Logger.Info("handshake successful", zap.String("peer", peerAddr))

	//torrentPlayer.peerChan <- &peer{
	//	conn:    peerConn,
	//	peerID:  res.peerID,
	//	options: res.options,
	//
	//	amChoking:      true,
	//	amInterested:   false,
	//	peerChoking:    true,
	//	peerInterested: false,
	//}
}

var (
	listenerInitFailed = errors.New("failed to create tcp listener")
)
