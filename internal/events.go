package internal

import "net"

const (
	tpEventNewPeer = iota
	tpEventDestroyPeer
	tpEventIncomingMessage
	tpEventOutgoingMessage
	tpPeerAvailableForDownloadEvent
	tpVerifyPieceEvent
	tpNewConnectionEvent
	tpUnchokeEvent
)

type event interface {
	eventType() int
}

type newPeerEvent struct {
	newPeer *peer
}

func (e *newPeerEvent) eventType() int { return tpEventNewPeer }

type destroyPeerEvent struct {
	peer *peer
}

func (e *destroyPeerEvent) eventType() int { return tpEventDestroyPeer }

type incomingMessageEvent struct {
	peer    *peer
	message peerwireMessage
}

func (e *incomingMessageEvent) eventType() int { return tpEventIncomingMessage }

type outgoingMessageEvent struct {
	peer    *peer
	message peerwireMessage
}

func (e *outgoingMessageEvent) eventType() int { return tpEventOutgoingMessage }

type peerAvailableForDownloadEvent struct {
	peer *peer
}

func (e *peerAvailableForDownloadEvent) eventType() int { return tpPeerAvailableForDownloadEvent }

type verifyPieceEvent struct {
	peer  *peer
	index int64
}

func (e *verifyPieceEvent) eventType() int { return tpVerifyPieceEvent }

type newConnectionEvent struct {
	conn *net.TCPConn
}

func (e *newConnectionEvent) eventType() int { return tpNewConnectionEvent }

type unchokeEvent struct{}

func (e *unchokeEvent) eventType() int { return tpUnchokeEvent }
