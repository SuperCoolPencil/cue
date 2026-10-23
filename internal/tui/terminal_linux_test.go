//go:build linux

package tui

import (
	"io"
	"os"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func queryTestTerminal(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { master.Close() })
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	number, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	name := "/dev/pts/" + strconv.Itoa(number)
	fd, err := unix.Open(name, unix.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	input := os.NewFile(uintptr(fd), name)
	t.Cleanup(func() { input.Close() })
	return input, master
}

func TestDetectTerminalBlockingStdin(t *testing.T) {
	for _, mode := range []string{"", "kitty"} {
		t.Run("mode="+mode, func(t *testing.T) {
			t.Setenv("CUE_IMAGES", mode)
			oldSupport, oldW, oldH := detectedKittyImage, kittyCellWpx, kittyCellHpx
			t.Cleanup(func() { detectedKittyImage, kittyCellWpx, kittyCellHpx = oldSupport, oldW, oldH })
			detectedKittyImage = nil
			kittyCellWpx, kittyCellHpx = 8, 16
			input, master := queryTestTerminal(t)
			if err := input.SetReadDeadline(time.Now()); err == nil {
				t.Fatal("expected blocking stdin without deadline support")
			}
			done := make(chan error, 1)
			go func() {
				query := make([]byte, len("\x1b_Gi=31,s=1,v=1,a=q,t=d,f=24;AAAA\x1b\\\x1b[c\x1b[16t"))
				if _, err := io.ReadFull(master, query); err != nil {
					done <- err
					return
				}
				_, err := master.Write([]byte("\x1b_Gi=31;OK\x1b\\\x1b[?1;2c\x1b[6;32;16t"))
				done <- err
			}()
			DetectTerminal(input, input)
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("terminal query was not sent")
			}
			if detectedKittyImage == nil || !*detectedKittyImage {
				t.Fatal("kitty support not detected")
			}
			if kittyCellWpx != 16 || kittyCellHpx != 32 {
				t.Fatalf("cell dimensions = %dx%d", kittyCellWpx, kittyCellHpx)
			}
		})
	}
}

func TestDetectTerminalTimeout(t *testing.T) {
	t.Setenv("CUE_IMAGES", "")
	old := detectedKittyImage
	t.Cleanup(func() { detectedKittyImage = old })
	input, _ := queryTestTerminal(t)
	started := time.Now()
	DetectTerminal(input, input)
	if time.Since(started) > time.Second {
		t.Fatal("query did not time out promptly")
	}
	if detectedKittyImage == nil || *detectedKittyImage {
		t.Fatal("unresponsive terminal should use ASCII")
	}
}
