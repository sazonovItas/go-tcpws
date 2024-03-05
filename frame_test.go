package gotcpws

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTcpFrameReaderFactory(t *testing.T) {
	msg := []byte{
		0x5A, 0xA5, 0x5A, 0xA5,
		0x81, 0x84, 0x0f, 0xff,
		0xff, 0x0f, 't', 'e', 's', 't',
	}
	readerFactory := tcpFrameReaderFactory{
		Reader: bufio.NewReader(bytes.NewReader(msg)),
	}

	reader, err := readerFactory.NewFrameReader()
	assert.Equal(t, nil, err, "should not be error creating reader from %s", msg)

	t.Run("check fin bit", func(t *testing.T) {
		got := reader.(*tcpFrameReader).header.Fin
		want := true

		assert.Equal(t, got, want, "got %d, want %d", got, want, msg)
	})

	t.Run("check opCode", func(t *testing.T) {
		got := reader.PayloadType()
		want := byte(0x1)

		assert.Equal(t, got, want, "got %d, want %d", got, want, msg)
	})

	t.Run("check payload length", func(t *testing.T) {
		got := reader.(*tcpFrameReader).header.Length
		want := int64(0x04)

		assert.Equal(t, got, want, "got %d, want %d", got, want, msg)
	})

	t.Run("check masking key", func(t *testing.T) {
		got := reader.(*tcpFrameReader).header.MaskingKey
		want := []byte{0x0f, 0xff, 0xff, 0x0f}

		assert.Equal(t, got, want, "got %d, want %d", got, want, msg)
	})

	t.Run("check frame length", func(t *testing.T) {
		got := reader.Len()
		want := len(msg) - 4

		assert.Equal(t, got, want, "got %d, want %d", got, want, msg)
	})

	t.Run("check paylod", func(t *testing.T) {
		got := make([]byte, 10)
		n, err := reader.Read(got)
		assert.Equal(t, nil, err, "should not have error")

		want := []byte("test")
		for i := 0; i < len(want); i++ {
			want[i] = want[i] ^ reader.(*tcpFrameReader).header.MaskingKey[i%4]
		}
		assert.Equal(t, got[:n], want, "got %s, want %s", got, want, msg)
	})

	t.Run("check header reader", func(t *testing.T) {
		headerReader := reader.HeaderReader()

		got := make([]byte, reader.(*tcpFrameReader).header.data.Len())
		n, err := headerReader.Read(got)
		assert.Equal(t, nil, err, "should not have error")

		want := []byte{
			0x81, 0x84, 0x0f, 0xff,
			0xff, 0x0f,
		}
		assert.Equal(t, got[:n], want, "got %s, want %s", got, want, msg)
	})
}

func TestTcpFrameWriterFactory(t *testing.T) {
	t.Run("check unmasked message", func(t *testing.T) {
		frameBuffer := make([]byte, 0, 32)
		buf := bytes.NewBuffer(frameBuffer)

		br := bufio.NewReader(buf)
		bw := bufio.NewWriter(buf)

		writerFabric := tcpFrameWriterFactory{
			needMaskingKey: false,
			Writer:         bw,
		}

		readerFactory := tcpFrameReaderFactory{
			Reader: br,
		}

		want := make([]byte, 10)
		_, _ = rand.Read(want)

		writer, err := writerFabric.NewFrameWriter(TextFrame)
		assert.Equal(t, nil, err, "should not be error creating writer")

		_, err = writer.Write(want)
		assert.Equal(t, nil, err, "should not be error write %d")

		got := make([]byte, 10)
		reader, _ := readerFactory.NewFrameReader()
		nr, _ := reader.Read(got)

		assert.Equal(t, want, got[:nr], "read after write should be equal messages")

		assert.Equal(t, writer.Close(), nil, "should be nil when close tcp writer")
	})

	t.Run("check masked message", func(t *testing.T) {
		frameBuffer := make([]byte, 0, 256)
		buf := bytes.NewBuffer(frameBuffer)

		br := bufio.NewReader(buf)
		bw := bufio.NewWriter(buf)

		writerFabric := tcpFrameWriterFactory{
			needMaskingKey: true,
			Writer:         bw,
		}

		readerFactory := tcpFrameReaderFactory{
			Reader: br,
		}

		want := make([]byte, 125)
		_, _ = rand.Read(want)

		writer, err := writerFabric.NewFrameWriter(TextFrame)
		assert.Equal(t, nil, err, "should not be error creating writer")

		_, err = writer.Write(want)
		assert.Equal(t, nil, err, "should not be error write to writer")

		got := make([]byte, 125)
		reader, _ := readerFactory.NewFrameReader()
		nr, _ := reader.Read(got)

		assert.Equal(t, want, got[:nr], "read after write should be equal messages")
		assert.Equal(t, writer.Close(), nil, "should be nil when close tcp writer")
	})
}

func TestDiffLenghtMsgReadWriteHandle(t *testing.T) {
	lengths := []int{125, 1024, 65535, 100000, 1000000}
	handler := &tcpFrameHandler{payloadType: TextFrame}

	for _, length := range lengths {
		t.Run("check unmasked message with length "+fmt.Sprint(length), func(t *testing.T) {
			frameBuffer := make([]byte, 0, length+14)
			buf := bytes.NewBuffer(frameBuffer)

			bw := bufio.NewWriter(buf)

			writerFabric := tcpFrameWriterFactory{
				needMaskingKey: false,
				Writer:         bw,
			}

			want := make([]byte, length)
			_, _ = rand.Read(want)

			writer, err := writerFabric.NewFrameWriter(TextFrame)
			assert.Equal(t, nil, err, "should not be error creating writer")

			nw, err := writer.Write(want)
			assert.Equal(t, nil, err, "should not be error write to writer")

			br := bufio.NewReader(buf)
			readerFactory := tcpFrameReaderFactory{
				Reader: br,
			}

			got := make([]byte, length)
			reader, _ := readerFactory.NewFrameReader()
			reader, err = handler.HandleFrame(reader)
			assert.Equal(t, nil, err, "should not be error handle frame")

			nr := 0
			for {
				i, err := reader.Read(got[nr:])
				nr += i
				if err == io.EOF {
					break
				}
			}

			assert.Equal(
				t,
				want,
				got[:nr],
				"read after write should be equal messages, want length %d, got length %d",
				nw,
				nr,
			)
		})

		t.Run("check masked message with length "+fmt.Sprint(length), func(t *testing.T) {
			frameBuffer := make([]byte, 0, length+18)
			buf := bytes.NewBuffer(frameBuffer)

			bw := bufio.NewWriter(buf)

			writerFabric := tcpFrameWriterFactory{
				needMaskingKey: true,
				Writer:         bw,
			}

			want := make([]byte, length)
			_, _ = rand.Read(want)

			writer, err := writerFabric.NewFrameWriter(TextFrame)
			assert.Equal(t, nil, err, "should not be error creating writer")

			nw, err := writer.Write(want)
			assert.Equal(t, nil, err, "should not be error write to writer")

			br := bufio.NewReader(buf)
			readerFactory := tcpFrameReaderFactory{
				Reader: br,
			}

			got := make([]byte, length)
			reader, _ := readerFactory.NewFrameReader()
			reader, err = handler.HandleFrame(reader)
			assert.Equal(t, nil, err, "should not be error handle frame")

			nr := 0
			for {
				i, err := reader.Read(got[nr:])
				nr += i
				if err == io.EOF {
					break
				}
			}

			assert.Equal(
				t,
				want,
				got[:nr],
				"read after write should be equal messages, want length %d, got length %d",
				nw,
				nr,
			)
		})
	}
}

func TestWriteClose(t *testing.T) {
	frameBuffer := make([]byte, 0, 14)
	buf := bytes.NewBuffer(frameBuffer)

	bw := bufio.NewWriter(buf)
	br := bufio.NewReader(buf)

	readerFactory := tcpFrameReaderFactory{
		Reader: br,
	}

	writerFactory := tcpFrameWriterFactory{
		Writer:         bw,
		needMaskingKey: false,
	}

	handler := &tcpFrameHandler{}
	handler.WriteClose(writerFactory, closeStatusNormal)

	rd, _ := readerFactory.NewFrameReader()
	_, err := handler.HandleFrame(rd)
	if assert.Error(
		t,
		err,
		"should be error on close frame with paylod type %d",
		handler.payloadType,
	) {
		assert.Equal(
			t,
			io.EOF,
			err,
			"should be EOR error on close frame with paylod type %d",
			handler.payloadType,
		)
	}

	want := make([]byte, 2)
	binary.BigEndian.PutUint16(want, uint16(closeStatusNormal))

	got := make([]byte, 14)
	n, _ := rd.Read(got)

	assert.Equal(t, want, got[:n], "close messages should be equal")
}

func Test_generateMaskingKey(t *testing.T) {
	maskingKey, err := generateMaskingKey()
	assert.Equal(t, nil, err, "generating mask should not create an error")

	assert.Equal(t, 4, len(maskingKey), "masking key should be length of 4")
}
