package gotcpws

import (
	"bufio"
	"io"
	"sync"
)

const (
	closeStatusNormal            = 1000
	closeStatusGoingAway         = 1001
	closeStatusProtocolError     = 1002
	closeStatusUnsupportedData   = 1003
	closeStatusFrameTooLarge     = 1004
	closeStatusNoStatusRcvd      = 1005
	closeStatusAbnormalClosure   = 1006
	closeStatusBadMessageData    = 1007
	closeStatusPolicyViolation   = 1008
	closeStatusTooBigData        = 1009
	closeStatusExtensionMismatch = 1010
)

// frameReader is interface to read ws like frame
type frameReader interface {
	// Reader is to read payload of the frame
	io.Reader

	// PayloadType returns payload type
	PayloadType() byte

	// HeaderReader returns a reader to read header of the frame
	HeaderReader() io.Reader

	// Len returns total len of the frame = header len + payload len
	Len() int
}

// frameReaderFactory is interface to create new frame reader
type frameReaderFactory interface {
	NewFrameReader() (r frameReader, err error)
}

// frameWriter is interface to write a ws like frame
type frameWriter interface {
	// Writer is to write a payload of a frame
	io.WriteCloser
}

// frameHandler is interface to handle different types of frame
type frameHandler interface {
	// handle different types of frame
	HandleFrame(frame frameReader) (r frameReader, err error)

	// write close frame with a status
	WriteClose(status int, writerFactory frameWriterFactory) (err error)
}

// frameWriterFactory is interface to create new frame writer
type frameWriterFactory interface {
	NewFrameWriter(payloadType byte) (w frameWriter, err error)
}

// TCPWSConn is struct for the
type TCPWSConn struct {
	buf *bufio.ReadWriter
	rwc io.ReadWriteCloser

	rio sync.Mutex
	frameReader
	frameReaderFactory

	wio sync.Mutex
	frameWriterFactory

	frameHandler
	defaultCloseStatus int

	// MaxPayloadBytes is max len of payload, if payload len
	// is greater than that len will return ErrFrameTooLarge
	MaxPayloadBytes int
}
