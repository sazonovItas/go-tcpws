package gotcpws

import (
	"bytes"
	cryptorand "crypto/rand"
	"fmt"
	"io"
	rand "math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type testConn struct {
	*bytes.Buffer
}

func (c testConn) Close() error { return nil }

func TestConnReadWriteClose(t *testing.T) {
	const length = 10000

	frameBuffer := make([]byte, 0, length)
	connBuffer := testConn{
		Buffer: bytes.NewBuffer(frameBuffer),
	}

	handler := &tcpFrameHandler{}
	conn := NewFrameConnection(connBuffer, nil, handler, 0, true)

	i := 0
	var want []byte

	t.Run("check write to connection", func(t *testing.T) {
		for {
			// Generate random message
			n := int(
				rand.Float64()*float64(
					length-i-maxHeaderLengthWithPreambule-1-minHeaderLengthWithPreambule-24,
				) + 1,
			)
			if n <= 0 {
				break
			}

			genData := make([]byte, n)
			nr, _ := cryptorand.Read(genData)

			// Write message
			nw, err := conn.Write(genData[:nr])
			t.Run(
				fmt.Sprintf("check write to connection from %d to %d message", i, i+nw),
				func(t *testing.T) {
					assert.Equal(t, nil, err, "should not be error to write")
				})

			// append data to check read after
			want = append(want, genData[:nr]...)
			i += nw
		}
	})

	// make buffer to read message
	var got []byte

	t.Run("check read from connectoin", func(t *testing.T) {
		j := 0
		buf := make([]byte, 1024)
		for j < len(want) {
			nr, err := conn.Read(buf)
			if !assert.Equal(t, nil, err, "should not be error read from connection") {
				return
			}

			t.Run(
				fmt.Sprintf("check read bytes with written bytes from %d to %d", j, j+nr),
				func(t *testing.T) {
					assert.Equal(t, want[j:j+nr], buf[:nr], "should be equal messages")
				},
			)

			j += nr
			got = append(got, buf[:nr]...)
		}

		assert.Equal(t, want, got, "should be equal messages")
	})

	t.Run("check close connection", func(t *testing.T) {
		buf := make([]byte, 16)

		conn.Close()
		_, err := conn.Read(buf)
		if assert.Error(t, err, "should error read on close connection") {
			assert.Equal(t, io.EOF, err, "should EOF error read on close connection")
		}
	})
}

func TestConnReadFrame(t *testing.T) {
	const length = 10000

	frameBuffer := make([]byte, 0, length)
	connBuffer := testConn{
		Buffer: bytes.NewBuffer(frameBuffer),
	}

	handler := &tcpFrameHandler{}
	conn := NewFrameConnection(connBuffer, nil, handler, 0, true)

	i := 0
	var want [][]byte
	for {
		// Generate random message
		n := int(
			rand.Float64()*float64(
				length-i-maxHeaderLengthWithPreambule-1-minHeaderLengthWithPreambule-6,
			) + 1,
		)
		if n <= 0 {
			break
		}

		genData := make([]byte, n)
		nr, _ := cryptorand.Read(genData)

		// Write message
		nw, _ := conn.Write(genData[:nr])

		// append data to check read after
		want = append(want, genData[:nr])
		i += nw
	}

	j := 0
	for j < len(want) {
		got, err := conn.ReadFrame()
		if !assert.Equal(t, nil, err, "error to read frame %d", j) {
			return
		}

		t.Run(
			fmt.Sprintf("check read frame want len %d and got len %d", len(want[j]), len(got)),
			func(t *testing.T) {
				assert.Equal(t, want[j], got, "should be equal messages")
			},
		)
		j++
	}

	t.Run("check err frame too large", func(t *testing.T) {
		conn.MaxPayloadBytes = 10

		msg := make([]byte, 12)
		_, _ = cryptorand.Read(msg)
		_, _ = conn.Write(msg)

		_, err := conn.ReadFrame()
		assert.Equal(t, ErrFrameTooLarge, err, "should be ErrFrameTooLarge error")
	})
}

func TestSetDeadline(t *testing.T) {
	frameBuffer := make([]byte, 0)
	connBuffer := testConn{
		Buffer: bytes.NewBuffer(frameBuffer),
	}

	handler := &tcpFrameHandler{}
	conn := NewFrameConnection(connBuffer, nil, handler, 0, true)

	t.Run("check set deadline for connection", func(t *testing.T) {
		err := conn.SetDeadline(time.Now())
		assert.Equal(t, errSetDeadline, err, "should be error to set deadline")
	})

	t.Run("check set read deadline for connection", func(t *testing.T) {
		err := conn.SetReadDeadline(time.Now())
		assert.Equal(t, errSetDeadline, err, "should be error to set deadline")
	})

	t.Run("check set write deadline for connection", func(t *testing.T) {
		err := conn.SetWriteDeadline(time.Now())
		assert.Equal(t, errSetDeadline, err, "should be error to set deadline")
	})
}
