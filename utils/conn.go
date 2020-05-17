package utils

import (
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/spf13/viper"
	"golang.org/x/crypto/ssh"
)

// SSHConnection handles state for a SSHConnection
type SSHConnection struct {
	SSHConn        *ssh.ServerConn
	Listeners      *sync.Map
	Close          chan bool
	Messages       chan string
	ProxyProto     byte
	Session        chan bool
	CleanupHandler bool
}

// SendMessage sends a console message to the connection
func (s *SSHConnection) SendMessage(message string, block bool) {
	if block {
		s.Messages <- message
		return
	}

	for i := 0; i < 5; {
		select {
		case <-s.Close:
			return
		case s.Messages <- message:
			return
		default:
			time.Sleep(100 * time.Millisecond)
			i++
		}
	}
}

// CleanUp closes all allocated resources and cleans them up
func (s *SSHConnection) CleanUp(state *State) {
	close(s.Close)
	s.SSHConn.Close()
	state.SSHConnections.Delete(s.SSHConn.RemoteAddr())
	log.Println("Closed SSH connection for:", s.SSHConn.RemoteAddr(), "user:", s.SSHConn.User())
}

// IdleTimeoutConn handles the connection with a context deadline
// code adapted from https://qiita.com/kwi/items/b38d6273624ad3f6ae79
type IdleTimeoutConn struct {
	Conn net.Conn
}

// Read is needed to implement the reader part
func (i IdleTimeoutConn) Read(buf []byte) (int, error) {
	err := i.Conn.SetReadDeadline(time.Now().Add(viper.GetDuration("idle-connection-timeout")))
	if err != nil {
		return 0, err
	}

	return i.Conn.Read(buf)
}

// Write is needed to implement the writer part
func (i IdleTimeoutConn) Write(buf []byte) (int, error) {
	err := i.Conn.SetWriteDeadline(time.Now().Add(viper.GetDuration("idle-connection-timeout")))
	if err != nil {
		return 0, err
	}

	return i.Conn.Write(buf)
}

// CopyBoth copies betwen a reader and writer
func CopyBoth(writer net.Conn, reader io.ReadWriteCloser) {
	closeBoth := func() {
		reader.Close()
		writer.Close()
	}

	var tcon io.ReadWriter

	if viper.GetBool("idle-connection") {
		tcon = IdleTimeoutConn{
			Conn: writer,
		}
	} else {
		tcon = writer
	}

	copyToReader := func() {
		_, err := io.Copy(reader, tcon)
		if err != nil && viper.GetBool("debug") {
			log.Println("Error copying to reader:", err)
		}

		closeBoth()
	}

	copyToWriter := func() {
		_, err := io.Copy(tcon, reader)
		if err != nil && viper.GetBool("debug") {
			log.Println("Error copying to writer:", err)
		}

		closeBoth()
	}

	go copyToReader()
	copyToWriter()
}
