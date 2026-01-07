package taskui

import (
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"

	"github.com/creack/pty"
	"github.com/go-chi/chi/v5"
	"golang.org/x/net/websocket"
)

// handleWebSocket returns a WebSocket handler for running tasks.
func (s *Server) handleWebSocket() http.Handler {
	return websocket.Handler(func(ws *websocket.Conn) {
		defer ws.Close()

		// Check if we can run
		name := chi.URLParam(ws.Request(), "name")
		if !s.SetRunning(name) {
			ws.Write([]byte("Another task is already running. Please wait.\r\n"))
			return
		}
		defer s.ClearRunning()

		ws.Write([]byte("Starting task: " + name + "\r\n\r\n"))

		if err := s.runTask(ws, name); err != nil {
			ws.Write([]byte("\r\n\033[31mError: " + err.Error() + "\033[0m\r\n"))
			return
		}

		ws.Write([]byte("\r\n\033[32mTask completed.\033[0m\r\n"))
	})
}

// runTask executes a task and streams output to the WebSocket.
func (s *Server) runTask(ws *websocket.Conn, taskName string) error {
	// Build xplat task command
	xplatBin, err := os.Executable()
	if err != nil {
		xplatBin = "xplat"
	}

	args := []string{"task", taskName}
	if s.config.Taskfile != "" && s.config.Taskfile != "Taskfile.yml" {
		args = append([]string{"task", "-t", s.config.Taskfile}, taskName)
	}

	cmd := exec.Command(xplatBin, args...)
	cmd.Dir = s.config.WorkDir
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"FORCE_COLOR=1",
	)

	// Use PTY on Unix for proper terminal handling
	if runtime.GOOS != "windows" {
		return s.runWithPTY(ws, cmd)
	}

	// On Windows, use simple pipe (no PTY support yet)
	return s.runWithPipes(ws, cmd)
}

// runWithPTY runs the command with a pseudo-terminal.
func (s *Server) runWithPTY(ws *websocket.Conn, cmd *exec.Cmd) error {
	// Start command with PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return err
	}
	defer ptmx.Close()

	// Copy PTY output to WebSocket
	done := make(chan struct{})
	go func() {
		io.Copy(ws, ptmx)
		close(done)
	}()

	// Copy WebSocket input to PTY (for interactive tasks)
	go func() {
		io.Copy(ptmx, ws)
	}()

	// Wait for command to finish
	err = cmd.Wait()
	<-done
	return err
}

// runWithPipes runs the command with simple pipes (Windows fallback).
func (s *Server) runWithPipes(ws *websocket.Conn, cmd *exec.Cmd) error {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	// Stream output
	done := make(chan struct{}, 2)
	go func() {
		io.Copy(ws, stdout)
		done <- struct{}{}
	}()
	go func() {
		io.Copy(ws, stderr)
		done <- struct{}{}
	}()

	<-done
	<-done

	return cmd.Wait()
}
