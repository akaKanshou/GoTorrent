package internal

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"slices"
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
		peerConn.Close()
		return
	} else if n != len(req.marshalMessage()) {
		logger.Logger.Error("could not send handshake", zap.String("peer", peerAddr), zap.Int("bytes written", n))
		peerConn.Close()
		return
	}

	peerConn.SetDeadline(time.Now().Add(time.Minute))
	responseBuf := make([]byte, handshakeLength)
	n, err = peerConn.Read(responseBuf)
	if err != nil {
		logger.Logger.Error("could not read handshake", zap.String("peer", peerAddr), zap.Error(err))
		peerConn.Close()
		return
	} else if n != handshakeLength {
		logger.Logger.Error("could not read handshake", zap.String("peer", peerAddr), zap.Int("bytes read", n))
		peerConn.Close()
		return
	}

	res := handshakeMessage{}
	err = res.unmarshalMessage(responseBuf)
	if err != nil {
		logger.Logger.Error("invalid handshake response", zap.String("peer", peerAddr), zap.Error(err))
		peerConn.Close()
	}
	logger.Logger.Info("handshake successful", zap.String("peer", peerAddr))

	peer := newPeer(peerConn, res.peerID, res.options,
		int(torrentPlayer.torrent.localData.infoDict.pieceLength), len(torrentPlayer.torrent.localData.infoDict.pieces))
	torrentPlayer.eventChan <- &newPeerEvent{
		newPeer: peer,
	}
	torrentPlayer.eventChan <- &outgoingMessageEvent{peer: peer,
		message: &bitfieldMessage{bitfieldData: torrentPlayer.fileManager.piecesDone.Bytes()},
	}
}

func (torrentPlayer *TorrentPlayer) handleHandshake(conn *net.TCPConn) {
	res := handshakeMessage{
		peerID:   torrentPlayer.peerID,
		infoHash: torrentPlayer.torrent.localData.infoHash,
	}

	responseBuf := make([]byte, handshakeLength)
	conn.SetDeadline(time.Now().Add(time.Minute))
	n, err := conn.Read(responseBuf)
	if err != nil {
		logger.Logger.Error("could not read handshake", zap.String("peer", conn.RemoteAddr().String()), zap.Error(err))
		conn.Close()
		return
	} else if n != handshakeLength {
		logger.Logger.Error("could not read handshake", zap.String("peer", conn.RemoteAddr().String()), zap.Int("bytes read", n))
		conn.Close()
		return
	}

	var req handshakeMessage
	err = req.unmarshalMessage(responseBuf)
	if err != nil {
		logger.Logger.Error("invalid handshake response", zap.String("peer", conn.RemoteAddr().String()), zap.Error(err))
		conn.Close()
		return
	}

	conn.SetDeadline(time.Now().Add(time.Minute))
	n, err = conn.Write(req.marshalMessage())
	if err != nil {
		logger.Logger.Error("could not send handshake", zap.String("peer", conn.RemoteAddr().String()), zap.Error(err))
		conn.Close()
		return
	} else if n != handshakeLength {
		logger.Logger.Error("could not write handshake", zap.String("peer", conn.RemoteAddr().String()), zap.Int("bytes written", n))
		conn.Close()
		return
	}
	conn.SetDeadline(time.Time{})

	peer := newPeer(conn, req.peerID, req.options&res.options,
		int(torrentPlayer.torrent.localData.infoDict.pieceLength), len(torrentPlayer.torrent.localData.infoDict.pieces))
	torrentPlayer.eventChan <- &newPeerEvent{
		newPeer: peer,
	}
}

func newPeer(conn net.Conn, peerID peerID, options uint64, pieceLength, numPieces int) *peer {
	peer := &peer{
		conn:    conn,
		peerID:  peerID,
		options: options,

		amChoking:      true,
		amInterested:   false,
		peerChoking:    true,
		peerInterested: false,

		piecesAvailable: bitfield.BigEndian.New(uint64(numPieces)),

		peerActivity:    make(chan any, 1),
		sendMessageChan: make(chan peerwireMessage, 64),

		destroyed: atomic.Bool{},

		pieceBuffer:      make([]byte, pieceLength),
		bytesRead:        bitfield.BigEndian.New(uint64(pieceLength)),
		pieceLength:      int64(pieceLength),
		pieceDownloading: -1,

		downloadRate: atomic.Int64{},
	}
	return peer
}

func (torrentPlayer *TorrentPlayer) writeToPeer(peer *peer) {
	defer torrentPlayer.Wg.Done()

	// TODO: Track number of pending messages per peer, and drop messages if it would block
	for {
		message, ok := <-peer.sendMessageChan
		if !ok {
			return
		}

		err := torrentPlayer.sendMessage(peer, message)
		if err != nil {
			peer.destroyed.Store(true)
			torrentPlayer.eventChan <- &destroyPeerEvent{peer: peer}
			return
		}
	}
}

func (torrentPlayer *TorrentPlayer) listenToPeer(peer *peer) {
	defer torrentPlayer.Wg.Done()

	buffer := make([]byte, 4, torrentPlayer.torrent.localData.infoDict.pieceLength)
	var err error
	for !peer.destroyed.Load() {
		logger.Logger.Debug("listening to peer", zap.String("peer", peer.conn.RemoteAddr().String()))
		err = readNBytes(buffer[:], peer.conn, 4)
		length := int(binary.BigEndian.Uint32(buffer[:4]))

		if len(buffer) < 4+length {
			buffer = append(buffer, make([]byte, length+4-len(buffer))...)
		}

		startTime := time.Now()
		err = readNBytes(buffer[4:], peer.conn, length)
		if err != nil {
			break
		}

		var message peerwireMessage
		message, err = parseMessage(buffer[:4+length], time.Since(startTime))
		if err != nil {
			break
		}

		logger.Logger.Debug("Message received", zap.String("peer", peer.conn.RemoteAddr().String()),
			zap.String("buffer", fmt.Sprintf("%x...", buffer[:min(4+length, 20)])))

		torrentPlayer.eventChan <- &incomingMessageEvent{
			peer:    peer,
			message: message,
		}
	}
	if err != nil {
		logger.Logger.Error("could not read from peer", zap.String("peer", peer.conn.RemoteAddr().String()), zap.Error(err))
	}

	peer.destroyed.Store(true)
	torrentPlayer.eventChan <- &destroyPeerEvent{peer: peer}
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
		if rPeer.pieceDownloading != -1 {
			delete(torrentPlayer.fileManager.pieceBuffers, rPeer.pieceDownloading)
			delete(torrentPlayer.fileManager.piecesInProgress, rPeer.pieceDownloading)
			rPeer.pieceDownloading = -1
		}
		rPeer.peerChoking = true
		logger.Logger.Debug("peer choked client", zap.String("peer", rPeer.conn.RemoteAddr().String()),
			zap.String("peerID", string(rPeer.peerID[:])))
		break
	case peerwireUnchokeID:
		// peer was choking, client was interested, now peer unchoked so is available to download from

		rPeer.peerChoking = false
		if rPeer.amInterested && rPeer.pieceDownloading == -1 {
			logger.Logger.Debug("Unchoked by a peer we are interested in", zap.String("peer", string(rPeer.peerID[:])))
			torrentPlayer.eventChan <- &peerAvailableForDownloadEvent{peer: rPeer}
		}

		logger.Logger.Debug("peer unchoked client", zap.String("peer", rPeer.conn.RemoteAddr().String()),
			zap.String("peerID", string(rPeer.peerID[:])))
		break
	case peerwireInterestedID:
		rPeer.peerInterested = true
		logger.Logger.Debug("peer interested in client", zap.String("peer", rPeer.conn.RemoteAddr().String()),
			zap.String("peerID", string(rPeer.peerID[:])))
		torrentPlayer.unchokePeersChan <- nil
		break
	case peerwireNotInterestedID:
		rPeer.peerInterested = false
		logger.Logger.Debug("peer not interested in client", zap.String("peer", rPeer.conn.RemoteAddr().String()),
			zap.String("peerID", string(rPeer.peerID[:])))
		torrentPlayer.unchokePeersChan <- nil
		break

	case peerwireHaveID:
		pwHaveMessage := message.(*haveMessage)
		err = rPeer.piecesAvailable.SetBit(uint64(pwHaveMessage.pieceIndex))
		if err != nil {
			return err
		}
		_, ok := torrentPlayer.fileManager.piecesInProgress[int64(pwHaveMessage.pieceIndex)]
		havePiece, _ := torrentPlayer.fileManager.piecesDone.TestBit(uint64(pwHaveMessage.pieceIndex))
		if !ok && !havePiece && !rPeer.amInterested {
			torrentPlayer.eventChan <- &outgoingMessageEvent{peer: rPeer, message: &interestedMessage{}}
		}
		break

	case peerwireBitfieldID:
		pwBitfieldMessage := message.(*bitfieldMessage)
		err = torrentPlayer.setBitfield(rPeer, pwBitfieldMessage.bitfieldData)
		if err != nil {
			return err
		}
		if !rPeer.amInterested && testInterested(torrentPlayer.fileManager.piecesDone, rPeer.piecesAvailable) {
			torrentPlayer.eventChan <- &outgoingMessageEvent{peer: rPeer, message: &interestedMessage{}}
		}
		if !rPeer.peerInterested && testInterested(rPeer.piecesAvailable, torrentPlayer.fileManager.piecesDone) {
			rPeer.peerInterested = true
		}
		logger.Logger.Debug("Setting bitfield", zap.String("peer", string(rPeer.peerID[:])), zap.String("bitfield",
			fmt.Sprintf("%x", rPeer.piecesAvailable.Bytes())))
		break

	case peerwirePieceID:
		pwPieceMessage := message.(*pieceMessage)
		rPeer.lastBlockUploadTime = time.Now()
		rPeer.uploadRate = int64(len(pwPieceMessage.block)) * int64(time.Second/pwPieceMessage.transmissionTime)
		pieceDone, err := rPeer.writeBlock(pwPieceMessage)
		if err != nil {
			return err
		}
		if pieceDone {
			torrentPlayer.eventChan <- &verifyPieceEvent{
				peer:  rPeer,
				index: rPeer.pieceDownloading,
			}
		}
		break

	case peerwireRequestID:
		requestMessage := message.(*requestMessage)
		havePiece, err := torrentPlayer.fileManager.piecesDone.TestBit(uint64(requestMessage.index))
		if err != nil {
			return err
		}
		if rPeer.amChoking {
			logger.Logger.Debug("Peer requesting piece is choked", zap.String("peer", fmt.Sprintf("%x", rPeer.peerID[:])))
			return nil
		}
		if !havePiece {
			logger.Logger.Debug("Piece requested by peer is not available on client", zap.String("peer", fmt.Sprintf("%x", rPeer.peerID[:])), zap.Int("index", requestMessage.index))
			return nil
		}
		go torrentPlayer.sendPiece(rPeer, requestMessage.index, requestMessage.begin, requestMessage.length)
		break

	case peerwireCancelID:
	case peerwirePortID:
		logger.Logger.Debug("Unimplemented message", zap.String("peer", string(rPeer.peerID[:])),
			zap.Uint8("messageID", message.messageType()))
		break
	case peerwireKeepAliveID:
		logger.Logger.Debug("Keep-alive message", zap.String("peer", string(rPeer.peerID[:])))
		break
	default:
		return invalidMessageIDError
	}

	// Update the lastCommunicated of the peer
	if !rPeer.destroyed.Load() {
		rPeer.peerActivity <- nil
	}
	return nil
}

func validateOutgoingMessage(rPeer *peer, message peerwireMessage) error {
	switch message.messageType() {
	case peerwireCancelID:
	case peerwireKeepAliveID:
	case peerwireBitfieldID:
		// No check
	case peerwirePortID:
		// Not implemented
		break

	case peerwireHaveID:
		haveMessage := message.(*haveMessage)
		peerHavePiece, _ := rPeer.piecesAvailable.TestBit(uint64(haveMessage.pieceIndex))
		if !rPeer.peerInterested && !peerHavePiece {
			rPeer.peerInterested = true
		}
		break

	case peerwireChokeID:
		if rPeer.amChoking {
			return peerIsAlreadyChoked
		}
		rPeer.amChoking = true
		break

	case peerwireUnchokeID:
		if !rPeer.amChoking {
			return peerIsAlreadyUnchoked
		}
		rPeer.amChoking = false
		break

	case peerwireInterestedID:
		if rPeer.amInterested {
			return peerKnowsInterest
		}
		rPeer.amInterested = true
		break

	case peerwireNotInterestedID:
		if !rPeer.amInterested {
			return peerKnowsDisinterest
		}
		rPeer.amInterested = false
		break

	case peerwirePieceID:
		if rPeer.amChoking {
			return peerIsChoked
		}
		break

	case peerwireRequestID:
		if rPeer.peerChoking {
			return peerIsChoking
		}
		break

	case peerwireMultiMessage:
		multiMessage := message.(*multiMessage)
		for i := range len(multiMessage.messages) {
			err := validateOutgoingMessage(rPeer, multiMessage.messages[i])
			if err != nil {
				multiMessage.messages[i] = nil
				logger.Logger.Debug("Dropped outgoing message", zap.String("peer", string(rPeer.peerID[:])), zap.String("message bytes", fmt.Sprintf("%x", message.marshalMessage())))
			}
		}
		break

	default:
		return invalidMessageIDError
	}

	return nil
}

func (torrentPlayer *TorrentPlayer) handleOutgoingMessage(rPeer *peer, message peerwireMessage) {
	err := validateOutgoingMessage(rPeer, message)
	if err != nil {
		logger.Logger.Debug("Dropped outgoing message", zap.String("peer", string(rPeer.peerID[:])), zap.String("message bytes", fmt.Sprintf("%x", message.marshalMessage())))
		return
	}

	// Update the lastCommunicated of the peer
	if !rPeer.destroyed.Load() {
		rPeer.sendMessageChan <- message
	}
}

var (
	peerKnowsInterest     = errors.New("dropped interested message; peer knows interest")
	peerKnowsDisinterest  = errors.New("dropped not interested message; peer knows disinterest")
	peerIsChoked          = errors.New("peer is choked; mustn't send piece")
	peerIsChoking         = errors.New("peer is choking; mustn't send request")
	peerIsAlreadyChoked   = errors.New("peer is already choked; not re-choking")
	peerIsAlreadyUnchoked = errors.New("peer is already choked; not re-unchoking")
)

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

func testInterested(bf, target *bitfield.BitField) bool {
	for i := range bf.Size() {
		iHavePiece, _ := bf.TestBit(uint64(i))
		uHavePiece, _ := target.TestBit(uint64(i))
		if !iHavePiece && uHavePiece {
			return true
		}
	}
	return false
}

var (
	listenerInitFailed = errors.New("failed to create tcp listener")

	bitfieldLengthInvalid = errors.New("bitfield length is invalid")
	bitfieldSpareBitsSet  = errors.New("bitfield spare bits set")
)

// sendMessage sends given message to rPeer.
// returns an error on failure, but is supposed to be ignored most of the time.
func (torrentPlayer *TorrentPlayer) sendMessage(rPeer *peer, message peerwireMessage) error {
	if rPeer.destroyed.Load() {
		logger.Logger.Warn("failed to send message; peer destroyed", zap.String("peer", string(rPeer.peerID[:])))
		return peerIsDestroyedErr
	}

	_, isPieceMessage := message.(*pieceMessage)

	sentBytes := message.marshalMessage()
	logger.Logger.Debug("Sending message", zap.String("peer", string(rPeer.peerID[:])), zap.String("bytes", fmt.Sprintf("%x", sentBytes)))

	rPeer.conn.SetWriteDeadline(time.Now().Add(90 * time.Second))
	defer rPeer.conn.SetWriteDeadline(time.Time{})

	startTime := time.Now()

	n, err := rPeer.conn.Write(sentBytes)
	if err != nil {
		logger.Logger.Error("Failed to send message", zap.String("peer", string(rPeer.peerID[:])), zap.Error(err))
		rPeer.destroyed.Store(true)
		torrentPlayer.eventChan <- &destroyPeerEvent{peer: rPeer}
		return err
	} else if n != len(sentBytes) {
		return io.ErrShortWrite
	}

	if isPieceMessage {
		rPeer.downloadRate.Store(int64(n) * int64(time.Second/time.Since(startTime)))
	}

	logger.Logger.Debug("Message sent", zap.String("peer", string(rPeer.peerID[:])), zap.String("bytes", fmt.Sprintf("%x", sentBytes)))

	if rPeer.destroyed.Load() {
		return peerIsDestroyedErr
	}
	rPeer.peerActivity <- nil
	return nil
}

var (
	peerIsDestroyedErr = errors.New("peer is destroyed")
)

func (p *peer) initBuffers(index, pieceLength int64) {
	p.pieceLength = pieceLength
	p.bytesRead = bitfield.BigEndian.New(uint64(pieceLength))
	p.pieceDownloading = index
	p.bytesDownloaded = 0
}

func (p *peer) writeBlock(message *pieceMessage) (bool, error) {
	if message.index != int(p.pieceDownloading) {
		return false, didNotAskForPieceError
	}
	for i := 0; i < len(message.block); i++ {
		haveByte, err := p.bytesRead.TestBit(uint64(i + message.begin))
		if err != nil {
			return false, err
		} else if !haveByte {
			p.bytesDownloaded++
		}
		p.bytesRead.SetBit(uint64(i + message.begin))
	}
	copy(p.pieceBuffer[message.begin:], message.block[:])
	return p.bytesDownloaded == p.pieceLength, nil
}

var (
	didNotAskForPieceError = errors.New("piece was not requested to peer")
	blockOutOfRangeError   = errors.New("block out of range of piece")
)

func filterInterestedPeers(peers []*peer) ([]*peer, []*peer) {
	interested, uninterested := make([]*peer, 0), make([]*peer, 0)
	for _, peer := range peers {
		if peer.peerInterested {
			interested = append(interested, peer)
		} else {
			uninterested = append(uninterested, peer)
		}
	}

	return interested, uninterested
}

func sortPeersForUnchokeAsLeecher(peers []*peer) []*peer {
	for _, peer := range peers {
		if time.Since(peer.lastBlockUploadTime) > 30*time.Second {
			peer.uploadRate = 0
		}
	}
	slices.SortFunc(peers, func(a, b *peer) int {
		if a.uploadRate == b.uploadRate {
			return 0
		} else if a.uploadRate < b.uploadRate {
			return -1
		} else {
			return 1
		}
	})
	return peers
}
