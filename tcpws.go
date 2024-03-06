package gotcpws

import (
	"bufio"
	"errors"
	"io"
	"net"
	"sync"
	"time"
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
	WriteClose(writerFactory frameWriterFactory, status int) (err error)
}

// frameWriterFactory is interface to create new frame writer
type frameWriterFactory interface {
	NewFrameWriter(payloadType byte) (w frameWriter, err error)
}

// Conn is struct for the
type Conn struct {
	buf *bufio.ReadWriter
	rwc io.ReadWriteCloser

	rio sync.Mutex
	frameReader
	frameReaderFactory

	wio sync.Mutex
	frameWriterFactory

	frameHandler
	PayloadType        byte
	defaultCloseStatus int

	// MaxPayloadBytes is max len of payload, if payload len
	// is greater than that len will return ErrFrameTooLarge
	MaxPayloadBytes int
}

// Read implements io.Reader interface
// it reads data of a frame from custom frame connection
// if msg is smaller than a frame size, the rest of a frame
// fills the msg and next Read will read next of the frame
func (conn *Conn) Read(msg []byte) (int, error) {
	conn.rio.Lock()
	defer conn.rio.Unlock()

	for {
		if conn.frameReader == nil {
			frame, err := conn.frameReaderFactory.NewFrameReader()
			if err != nil {
				return 0, err
			}

			// handle frame
			conn.frameReader, err = conn.frameHandler.HandleFrame(frame)
			if err != nil {
				return 0, err
			}

			// if frameReader is nil, create new reader
			if conn.frameReader == nil {
				continue
			}
		}

		n, err := conn.frameReader.Read(msg)
		if err == io.EOF {
			conn.frameReader = nil
			continue
		}

		return n, err
	}
}

// ReadFrame reads all frame of the connection
// if frame is too large return nil, ErrFrameTooLarge
func (conn *Conn) ReadFrame() ([]byte, error) {
	conn.rio.Lock()
	defer conn.rio.Unlock()

	// finish reading frameReader if it exists
	if conn.frameReader != nil {
		_, err := io.Copy(io.Discard, conn.frameReader)
		if err != nil {
			return nil, err
		}
		conn.frameReader = nil
	}

	for {
		frame, err := conn.frameReaderFactory.NewFrameReader()
		if err != nil {
			return nil, err
		}

		frame, err = conn.frameHandler.HandleFrame(frame)
		if err != nil {
			return nil, err
		}

		if frame == nil {
			continue
		}

		maxPayloadBytes := conn.MaxPayloadBytes
		if maxPayloadBytes == 0 {
			maxPayloadBytes = DefaultMaxPayloadBytes
		}

		// check payload size if we can
		if r, ok := frame.(*tcpFrameReader); ok && maxPayloadBytes < int(r.header.Length) {
			// finish reading frame
			_, err := io.Copy(io.Discard, frame)
			if err != nil {
				return nil, err
			}

			return nil, ErrFrameTooLarge
		}

		data, err := io.ReadAll(frame)
		return data, err
	}
}

// Write implemets io.Writer interface
// write data as a custom frame of framing connection
func (conn *Conn) Write(msg []byte) (int, error) {
	conn.wio.Lock()
	defer conn.wio.Unlock()

	w, err := conn.frameWriterFactory.NewFrameWriter(conn.PayloadType)
	if err != nil {
		return 0, err
	}
	defer w.Close()

	n, err := w.Write(msg)
	return n, err
}

var errSetDeadline = errors.New("conn: cannot set deadline: not using new.Conn")

// Close implements io.Closer interface
// send close frame and close rwc
func (conn *Conn) Close() error {
	err := conn.frameHandler.WriteClose(conn.frameWriterFactory, conn.defaultCloseStatus)
	err1 := conn.rwc.Close()
	if err != nil {
		return err
	}

	return err1
}

// SetDeadline sets connection's read & write deadline
func (conn *Conn) SetDeadline(t time.Time) error {
	if c, ok := conn.rwc.(net.Conn); ok {
		return c.SetDeadline(t)
	}

	return errSetDeadline
}

// SetDeadline sets connection read deadline
func (conn *Conn) SetReadDeadline(t time.Time) error {
	if c, ok := conn.rwc.(net.Conn); ok {
		return c.SetReadDeadline(t)
	}

	return errSetDeadline
}

// SetDeadline sets connection write deadline
func (conn *Conn) SetWriteDeadline(t time.Time) error {
	if c, ok := conn.rwc.(net.Conn); ok {
		return c.SetWriteDeadline(t)
	}

	return errSetDeadline
}
