package socket

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

var UDSPath = os.TempDir() + "/whats4linux.sock"

// CommandHandler is invoked for socket commands that aren't handled by the
// built-in window commands (show/hide/quit). It returns the reply to send back
// on the connection ("" for none) and whether the command was recognised.
type CommandHandler func(cmd string) (reply string, handled bool)

type UnixSocket struct {
	mu      sync.Mutex
	ctx     context.Context
	conn    net.Conn
	handler CommandHandler
}

func NewUnixSocket(ctx context.Context) (*UnixSocket, error) {
	return &UnixSocket{
		ctx: ctx,
	}, nil
}

// SetCommandHandler registers a handler for app-specific commands (e.g. the
// systray notification toggle). Must be set before ListenAndServe.
func (s *UnixSocket) SetCommandHandler(handler CommandHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handler = handler
}

func (s *UnixSocket) ListenAndServe() error {
	if err := os.RemoveAll(UDSPath); err != nil {
		return err
	}
	listener, err := net.Listen("unix", UDSPath)
	if err != nil {
		return err
	}
	defer listener.Close()
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("accept error:", err)
			continue
		}
		s.handleConn(conn)
	}
}

// handleConn serves a single client (the systray) over a persistent
// connection. Commands are newline-delimited; replies/pushes are written back
// on the same connection via SendCommand.
func (s *UnixSocket) handleConn(conn net.Conn) {
	s.mu.Lock()
	s.conn = conn
	handler := s.handler
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		if s.conn == conn {
			s.conn = nil
		}
		s.mu.Unlock()
		conn.Close()
	}()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		msg := scanner.Text()
		if msg == "" {
			continue
		}

		switch msg {
		case "show":
			runtime.WindowUnminimise(s.ctx)
			runtime.Show(s.ctx)
		case "hide":
			runtime.Hide(s.ctx)
		case "quit":
			log.Println("Quit signal received from systray")
			runtime.Quit(s.ctx)
		default:
			if handler != nil {
				if reply, handled := handler(msg); handled {
					if reply != "" {
						if err := s.SendCommand(reply); err != nil {
							log.Println("socket reply error:", err)
						}
					}
					continue
				}
			}
			fmt.Println("unknown command:", msg)
		}
	}
	if err := scanner.Err(); err != nil {
		log.Println("read error:", err)
	}
}

// SendCommand writes a newline-framed message to the connected client.
func (s *UnixSocket) SendCommand(cmd string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		return fmt.Errorf("not connected to socket")
	}
	_, err := s.conn.Write([]byte(cmd + "\n"))
	if err != nil {
		defer s.conn.Close()
		s.conn = nil
	}
	return err
}
