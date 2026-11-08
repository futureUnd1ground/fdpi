package main

import (
	"context"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func benchBinary(b *testing.B) string {
	b.Helper()
	path := filepath.Join(b.TempDir(), "fdpi")
	if out, e := exec.Command("go", "build", "-o", path, ".").CombinedOutput(); e != nil {
		b.Fatalf("%v %s", e, out)
	}
	return path
}
func BenchmarkCLIStartup(b *testing.B) {
	binary := benchBinary(b)
	var rss int64
	b.ResetTimer()
	for b.Loop() {
		cmd := exec.Command(binary, "--version")
		if out, e := cmd.Output(); e != nil || !strings.Contains(string(out), "FDPI") {
			b.Fatalf("%v %s", e, out)
		}
		if u, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage); ok && u.Maxrss > rss {
			rss = u.Maxrss
		}
	}
	b.ReportMetric(float64(rss), "maxRSS-KiB")
}
func BenchmarkTUIStartupPTY(b *testing.B) {
	binary := benchBinary(b)
	dir := b.TempDir()
	var first int64
	var rss int64
	b.ResetTimer()
	for b.Loop() {
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		master, e := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
		if e != nil {
			cancel()
			b.Fatal(e)
		}
		if e = unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); e != nil {
			master.Close()
			cancel()
			b.Fatal(e)
		}
		number, e := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
		if e != nil {
			master.Close()
			cancel()
			b.Fatal(e)
		}
		slave, e := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR|syscall.O_NOCTTY, 0)
		if e != nil {
			master.Close()
			cancel()
			b.Fatal(e)
		}
		_ = unix.IoctlSetWinsize(int(slave.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Row: 25, Col: 80})
		cmd := exec.CommandContext(ctx, binary, "tui")
		cmd.Env = append(os.Environ(), "TERM=xterm-256color", "XDG_CONFIG_HOME="+dir)
		cmd.Stdin = slave
		cmd.Stdout = slave
		cmd.Stderr = slave
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
		if e = cmd.Start(); e != nil {
			slave.Close()
			master.Close()
			cancel()
			b.Fatal(e)
		}
		_ = slave.Close()
		// Command timeout closes the slave through process exit, bounding a failed render.
		var output strings.Builder
		buffer := make([]byte, 4096)
		rendered := false
		backgroundAnswered, cursorAnswered := false, false
		for output.Len() < 1<<20 {
			n, re := master.Read(buffer)
			if n > 0 {
				output.Write(buffer[:n])
				if !backgroundAnswered && strings.Contains(output.String(), "\x1b]11;?") {
					_, _ = master.Write([]byte("\x1b]11;rgb:1e1e/1e1e/2e2e\x1b\\"))
					backgroundAnswered = true
				}
				if !cursorAnswered && strings.Contains(output.String(), "\x1b[6n") {
					_, _ = master.Write([]byte("\x1b[1;1R"))
					cursorAnswered = true
				}
				if strings.Contains(output.String(), "◈ FDPI") {
					first += time.Since(start).Nanoseconds()
					rendered = true
					break
				}
			}
			if re != nil {
				break
			}
		}
		_, _ = master.Write([]byte("q"))
		e = cmd.Wait()
		slave.Close()
		master.Close()
		cancel()
		if !rendered || e != nil {
			b.Fatalf("TUI startup: rendered=%t error=%v output=%s", rendered, e, output.String())
		}
		if u, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage); ok && u.Maxrss > rss {
			rss = u.Maxrss
		}
	}
	b.ReportMetric(float64(first)/float64(b.N), "first-frame-ns")
	b.ReportMetric(float64(rss), "maxRSS-KiB")
}
