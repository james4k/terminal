package terminal

import (
	"testing"
)

func TestCSIParse(t *testing.T) {
	var csi csiEscape
	csi.reset()
	csi.buf = []byte("s")
	csi.parse()
	if csi.mode != 's' || len(csi.args) != 0 {
		t.Fatal("CSI parse failed")
	}

	csi.reset()
	csi.buf = []byte("31T")
	csi.parse()
	if csi.mode != 'T' || len(csi.args) != 1 || csi.args[0] != 31 {
		t.Fatal("CSI parse failed")
	}

	csi.reset()
	csi.buf = []byte("48;2f")
	csi.parse()
	if csi.mode != 'f' || len(csi.args) != 2 || csi.args[0] != 48 || csi.args[1] != 2 {
		t.Fatal("CSI parse failed")
	}

	csi.reset()
	csi.buf = []byte("?25l")
	csi.parse()
	if csi.mode != 'l' || len(csi.args) != 1 || csi.args[0] != 25 || csi.priv != true {
		t.Fatal("CSI parse failed")
	}
}
