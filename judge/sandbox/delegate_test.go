package sandbox

import (
	"os"
	"strconv"
	"testing"
)

func TestIDMaps(t *testing.T) {
	uid, gid := idMaps()
	wantUID, wantGID := "65534:65534:1", "65534:65534:1"
	if os.Geteuid() != 0 {
		wantUID = "65534:" + strconv.Itoa(os.Geteuid()) + ":1"
		wantGID = "65534:" + strconv.Itoa(os.Getegid()) + ":1"
	}
	if uid != wantUID || gid != wantGID {
		t.Errorf("idMaps() = %q, %q; want %q, %q", uid, gid, wantUID, wantGID)
	}
	if Rootless() != (os.Geteuid() != 0) {
		t.Error("Rootless disagrees with Geteuid")
	}
}
