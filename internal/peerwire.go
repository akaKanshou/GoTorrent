package internal

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"time"

	"gitlab.com/akaKanshou/gotorrent/logger"
	"go.loafoe.dev/bitfield/v2"
	"go.uber.org/zap"
)

const (
	maxTCPBufferSize = 4096
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

	torrentPlayer.acceptPeers(&peer{
		conn:    peerConn,
		peerID:  res.peerID,
		options: res.options,

		amChoking:      true,
		amInterested:   false,
		peerChoking:    true,
		peerInterested: false,

		piecesAvailable: bitfield.BigEndian.New(uint64(len(torrentPlayer.torrent.localData.infoDict.pieces))),

		peerActivity: make(chan any),

		destroyed: atomic.Bool{},
	})
}

func (torrentPlayer *TorrentPlayer) listenToPeer(peer *peer) {
	defer torrentPlayer.Wg.Done()

	buffer := make([]byte, 4, torrentPlayer.torrent.localData.infoDict.pieceLength)
	var err error
	logger.Logger.Debug("listening to peer", zap.String("peer", peer.conn.RemoteAddr().String()))
	for {
		err = readNBytes(buffer[:], peer.conn, 4)
		length := int(binary.BigEndian.Uint32(buffer[:4]))

		if len(buffer) < 4+length {
			buffer = append(buffer, make([]byte, length+4-len(buffer))...)
		}

		err = readNBytes(buffer[4:], peer.conn, length)
		if err != nil {
			break
		}

		message, err := parseMessage(buffer[:4+length])
		if err != nil {
			break
		}

		logger.Logger.Debug("Message received", zap.String("peer", peer.conn.RemoteAddr().String()),
			zap.String("buffer", fmt.Sprintf("%x", buffer[:4+length])))

		err = torrentPlayer.handleMessage(peer, message)
		if err != nil {
			break
		}
	}
	logger.Logger.Error("could not read from peer", zap.String("peer", peer.conn.RemoteAddr().String()), zap.Error(err))
}

func readNBytes(buffer []byte, conn net.Conn, length int) error {
	seek := 0
	for seek != length {
		n, err := conn.Read(buffer[seek:length])
		if err != nil {
			return err
		}
		seek += n
	}
	return nil
}

func (torrentPlayer *TorrentPlayer) handleMessage(rPeer *peer, message peerwireMessage) error {
	var err error
	switch message.messageType() {
	case peerwireChokeID:
		rPeer.peerChoking = true
		logger.Logger.Debug("peer choked client", zap.String("peer", rPeer.conn.RemoteAddr().String()),
			zap.String("peerID", string(rPeer.peerID[:])))
		break
	case peerwireUnchokeID:
		rPeer.peerChoking = false
		logger.Logger.Debug("peer unchoked client", zap.String("peer", rPeer.conn.RemoteAddr().String()),
			zap.String("peerID", string(rPeer.peerID[:])))
		break
	case peerwireInterestedID:
		rPeer.peerInterested = true
		logger.Logger.Debug("peer interested in client", zap.String("peer", rPeer.conn.RemoteAddr().String()),
			zap.String("peerID", string(rPeer.peerID[:])))
		break
	case peerwireNotInterestedID:
		rPeer.peerInterested = false
		logger.Logger.Debug("peer not interested in client", zap.String("peer", rPeer.conn.RemoteAddr().String()),
			zap.String("peerID", string(rPeer.peerID[:])))
		break
	case peerwireHaveID:
		pwHaveMessage := message.(*haveMessage)
		err = rPeer.piecesAvailable.SetBit(uint64(pwHaveMessage.pieceIndex))
		break
	case peerwireBitfieldID:
		pwBitfieldMessage := message.(*bitfieldMessage)
		err = torrentPlayer.setBitfield(rPeer, pwBitfieldMessage.bitfieldData)
		logger.Logger.Debug("Setting bitfield", zap.String("peer", string(rPeer.peerID[:])), zap.String("bitfield",
			fmt.Sprintf("%x", rPeer.piecesAvailable.Bytes())))
		break
	case peerwireRequestID:
	case peerwirePieceID:
	case peerwireCancelID:
	case peerwirePortID:
		logger.Logger.Debug("Unimplemented message", zap.String("peer", string(rPeer.peerID[:])),
			zap.Uint8("messageID", message.messageType()))
		break
	case peerwireKeepAliveID:
		logger.Logger.Debug("Keep-alive message", zap.String("peer", string(rPeer.peerID[:])))
		break
	}

	if err != nil {
		rPeer.destroyed.Store(true)
		torrentPlayer.eventChan <- &destroyPeerEvent{rPeer}
		return err
	}

	// Update the lastCommunicated of the peer
	if !rPeer.destroyed.Load() {
		rPeer.peerActivity <- nil
	}
	return nil
}

func (torrentPlayer *TorrentPlayer) setBitfield(rPeer *peer, pwBitfield []byte) error {
	bfLength, numPieces := len(pwBitfield), len(torrentPlayer.torrent.localData.infoDict.pieces)

	// Check if length of pwBitField is valid
	// ceiling(numPieces/8) == bitfieldLength
	if bfLength != (numPieces+7)/8 {
		return bitfieldLengthInvalid
	}

	// Check if spare of pwBitFields are not set
	nSpareBits := (8 - numPieces%8) % 8
	spareBits := ((1 << nSpareBits) - 1) & pwBitfield[bfLength-1]
	if spareBits != 0 {
		return bitfieldSpareBitsSet
	}

	// Set all bits
	rPeer.piecesAvailable = bitfield.BigEndian.FromBytes(
		pwBitfield,
		uint64(len(torrentPlayer.torrent.localData.infoDict.pieces)),
	)

	return nil
}

var (
	listenerInitFailed = errors.New("failed to create tcp listener")

	bitfieldLengthInvalid = errors.New("bitfield length is invalid")
	bitfieldSpareBitsSet  = errors.New("bitfield spare bits set")
)
