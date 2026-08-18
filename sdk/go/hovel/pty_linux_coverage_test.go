//go:build linux

package hovel

import (
	"errors"
	"os"
	"testing"
)

func TestOpenPTYPlatformFailureBranches(t *testing.T) {
	want := errors.New("injected")
	base := ptyPlatformOps{
		open:      func(string, int, uint32) (int, error) { return -1, want },
		close:     func(int) error { return nil },
		unlock:    func(int) error { return nil },
		slaveName: func(int) (string, error) { return "/dev/pts/test", nil },
	}
	if _, _, _, err := openPTYPlatformWithOps(base); err == nil {
		t.Fatal("master open failure was ignored")
	}

	master, err := os.OpenFile("/dev/null", os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	masterFD := int(master.Fd())
	master.Close()
	openCount := 0
	ops := base
	ops.open = func(string, int, uint32) (int, error) { openCount++; return masterFD, nil }
	ops.unlock = func(int) error { return want }
	if _, _, _, err := openPTYPlatformWithOps(ops); err == nil {
		t.Fatal("unlock failure was ignored")
	}

	openCount = 0
	ops.unlock = func(int) error { return nil }
	ops.slaveName = func(int) (string, error) { return "", want }
	if _, _, _, err := openPTYPlatformWithOps(ops); err == nil {
		t.Fatal("slave-name failure was ignored")
	}

	openCount = 0
	ops.slaveName = func(int) (string, error) { return "/dev/pts/test", nil }
	ops.open = func(string, int, uint32) (int, error) {
		openCount++
		if openCount == 1 {
			return masterFD, nil
		}
		return -1, want
	}
	if _, _, _, err := openPTYPlatformWithOps(ops); err == nil {
		t.Fatal("input open failure was ignored")
	}

	openCount = 0
	ops.open = func(string, int, uint32) (int, error) {
		openCount++
		if openCount < 3 {
			return masterFD, nil
		}
		return -1, want
	}
	ops.close = func(int) error { return want }
	if _, _, _, err := openPTYPlatformWithOps(ops); err == nil {
		t.Fatal("output open failure was ignored")
	}
}

func TestPTYIoctlFailureBranches(t *testing.T) {
	if err := unlockPTY(-1); err == nil {
		t.Fatal("unlockPTY(-1) succeeded")
	}
	if _, err := ptsName(-1); err == nil {
		t.Fatal("ptsName(-1) succeeded")
	}
}
