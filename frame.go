package gotcpws

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
)

const (
	ContinuationFrame = 0
	TextFrame         = 1
	BinaryFrame       = 2
	CloseFrame        = 8
	UnknownFrame      = 255

	DefaultMaxPayloadBytes = 32 << 20 // 32MB

	maxHeaderLengthWithPreambule = 18
	minHeaderLengthWithPreambule = 6
)

var (
	// preambule adds at the start of each frame
	preambule = []byte{0x5A, 0xA5, 0x5A, 0xA5}

	ErrBadPreambule  = errors.New("error bad preambule")
	ErrBadHeader     = errors.New("error bad header")
	ErrBadMaskingKey = errors.New("bad masking key")
	ErrFrameTooLarge = errors.New("error frame is too large")
)

// tcpFrameHeader is header of the frame (without preambule)
type tcpFrameHeader struct {
	Fin        bool
	Rsv        [3]bool
	OpCode     byte
	Length     int64
	MaskingKey []byte

	data *bytes.Buffer
}

type tcpFrameReader struct {
	reader io.Reader

	header tcpFrameHeader
	pos    int64
	length int
}

func (frame *tcpFrameReader) Read(msg []byte) (int, error) {
	n, err := frame.reader.Read(msg)
	if frame.header.MaskingKey != nil {
		for i := 0; i < n; i++ {
			msg[i] ^= frame.header.MaskingKey[frame.pos%4]
			frame.pos++
		}
	}

	return n, err
}

func (frame *tcpFrameReader) PayloadType() byte {
	return frame.header.OpCode
}

func (frame *tcpFrameReader) HeaderReader() io.Reader {
	if frame.header.data == nil {
		return nil
	}

	if frame.header.data.Len() == 0 {
		return nil
	}

	return frame.header.data
}

func (frame *tcpFrameReader) Len() int {
	return frame.length
}

type tcpFrameReaderFactory struct {
	*bufio.Reader
}

// NewFrameReader reads header of a frame and creates new frameReader
// If while reading header occured error return nil, err
func (buf tcpFrameReaderFactory) NewFrameReader() (frameReader, error) {
	tcpFrame := new(tcpFrameReader)

	// check preambule of a frame
	for i := range preambule {
		b, err := buf.ReadByte()
		if err != nil {
			return nil, err
		}

		if b != preambule[i] {
			return nil, ErrBadPreambule
		}
	}

	var (
		b      byte
		header []byte
		err    error
	)

	// Read Fin, RSV1, RSV2, RSV3 bits
	b, err = buf.ReadByte()
	if err != nil {
		return nil, err
	}

	header = append(header, b)
	tcpFrame.header.Fin = (b & 0x80) != 0
	for i := 0; i < 3; i++ {
		shift := uint(6 - i)
		tcpFrame.header.Rsv[i] = ((b >> shift) & 1) != 0
	}
	tcpFrame.header.OpCode = b & 0x0f

	// read payload len
	b, err = buf.ReadByte()
	if err != nil {
		return nil, err
	}

	header = append(header, b)
	mask := (b & 0x80) != 0
	b &= 0x7f

	// check size of payload to get next length of next field
	lengthFields := 0
	switch {
	case b <= 125:
		tcpFrame.header.Length = int64(b)
	case b == 126:
		lengthFields = 2
	case b == 127:
		lengthFields = 8
	}

	for i := 0; i < lengthFields; i++ {
		b, err = buf.ReadByte()
		if err != nil {
			return nil, err
		}

		header = append(header, b)
		tcpFrame.header.Length = tcpFrame.header.Length*256 + int64(b)
	}

	// check mask's bytes if it exists
	if mask {
		for i := 0; i < 4; i++ {
			b, err = buf.ReadByte()
			if err != nil {
				return nil, err
			}

			header = append(header, b)
			tcpFrame.header.MaskingKey = append(tcpFrame.header.MaskingKey, b)
		}
	}

	tcpFrame.header.data = bytes.NewBuffer(header)
	tcpFrame.length = len(header) + int(tcpFrame.header.Length)
	tcpFrame.reader = io.LimitReader(buf.Reader, tcpFrame.header.Length)
	return tcpFrame, nil
}

type tcpFrameWriter struct {
	writer *bufio.Writer

	header *tcpFrameHeader
}

// For io.WriterCloser interface
func (frame *tcpFrameWriter) Close() error { return nil }

// Writer msg to connection and return amount of bytes was
// written + len(preambule) + len(header) and error
func (frame *tcpFrameWriter) Write(msg []byte) (int, error) {
	var (
		b      byte
		header []byte
		err    error
	)

	if frame.header.Fin {
		b |= 0x80
	}

	for i := 0; i < 3; i++ {
		if frame.header.Rsv[i] {
			shift := uint(6 - i)
			b |= 1 << shift
		}
	}
	b |= frame.header.OpCode
	header = append(header, b)

	b = 0x00
	if frame.header.MaskingKey != nil {
		b = 0x80
	}

	// write payload len
	lengthFields := 0
	length := len(msg)
	switch {
	case length <= 125:
		b |= byte(length)
	case length < 65536:
		b |= 126
		lengthFields = 2
	default:
		b |= 127
		lengthFields = 8
	}
	header = append(header, b)

	if lengthFields == 2 {
		header = binary.BigEndian.AppendUint16(header, uint16(length))
	}

	if lengthFields == 8 {
		header = binary.BigEndian.AppendUint64(header, uint64(length))
	}

	if frame.header.MaskingKey != nil {
		if len(frame.header.MaskingKey) != 4 {
			return 0, ErrBadMaskingKey
		}

		header = append(header, frame.header.MaskingKey...)
		data := make([]byte, len(msg))
		for i := range data {
			data[i] = msg[i] ^ frame.header.MaskingKey[i%4]
		}
		_, _ = frame.writer.Write(preambule)
		_, _ = frame.writer.Write(header)
		_, _ = frame.writer.Write(data)
		err = frame.writer.Flush()
		return len(preambule) + len(header) + len(msg), err
	}

	_, _ = frame.writer.Write(preambule)
	_, _ = frame.writer.Write(header)
	_, _ = frame.writer.Write(msg)
	err = frame.writer.Flush()
	return len(preambule) + len(header) + len(msg), err
}

// tcpFrameWriterFactory creates writer for a frame
// if needMaskingKey is true, a payload will masking
type tcpFrameWriterFactory struct {
	*bufio.Writer
	needMaskingKey bool
}

func (buf tcpFrameWriterFactory) NewFrameWriter(payloadType byte) (frameWriter, error) {
	frameHeader := &tcpFrameHeader{Fin: true, OpCode: payloadType}
	if buf.needMaskingKey {
		var err error
		frameHeader.MaskingKey, err = generateMaskingKey()
		if err != nil {
			return nil, err
		}
	}

	return &tcpFrameWriter{writer: buf.Writer, header: frameHeader}, nil
}

type tcpFrameHandler struct {
	payloadType byte
}

func (handler *tcpFrameHandler) HandleFrame(frame frameReader) (frameReader, error) {
	switch frame.PayloadType() {
	case ContinuationFrame:
		frame.(*tcpFrameReader).header.OpCode = handler.payloadType
	case TextFrame, BinaryFrame:
		handler.payloadType = frame.PayloadType()
	case CloseFrame:
		return nil, io.EOF
	}

	return frame, nil
}

func (handler *tcpFrameHandler) WriteClose(writerFactory frameWriterFactory, status int) error {
	writer, err := writerFactory.NewFrameWriter(CloseFrame)
	if err != nil {
		return err
	}

	_, err = writer.Write(binary.BigEndian.AppendUint16([]byte{}, uint16(status)))
	return err
}

// create new tcp frame connection from rwc interface
// rwc - readWriteCloser interface
// if buf - nil create new bufio readWriter from rwc
// handler - handles frame header and close connection
// maxPayloadBytes - max size of the message
func NewFrameConnection(
	rwc io.ReadWriteCloser,
	buf *bufio.ReadWriter,
	handler frameHandler,
	maxPayloadBytes int,
) *Conn {
	if buf == nil {
		br := bufio.NewReader(rwc)
		bw := bufio.NewWriter(rwc)
		buf = bufio.NewReadWriter(br, bw)
	}

	conn := &Conn{
		buf:                buf,
		rwc:                rwc,
		frameReaderFactory: &tcpFrameReaderFactory{Reader: buf.Reader},
		frameWriterFactory: &tcpFrameWriterFactory{Writer: buf.Writer},
		frameHandler:       handler,
		defaultCloseStatus: closeStatusNormal,
		MaxPayloadBytes:    maxPayloadBytes,
	}
	return conn
}

// Generate 4 byte masking key for a frame
func generateMaskingKey() ([]byte, error) {
	maskingKey := make([]byte, 4)
	_, err := rand.Read(maskingKey)
	return maskingKey, err
}
