package capture

import (
	"encoding/binary"
	"golang.org/x/net/bpf"
	"testing"
)

func TestServiceFilter(t *testing.T) {
	raw, e := ServiceFilter([]int{80, 50051})
	if e != nil {
		t.Fatal(e)
	}
	ins, ok := bpf.Disassemble(raw)
	if !ok {
		t.Fatal("disassemble")
	}
	vm, e := bpf.NewVM(ins)
	if e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		ether    uint16
		proto    byte
		src, dst uint16
		want     bool
	}{{0x0800, 6, 1234, 80, true}, {0x0800, 17, 50051, 9, true}, {0x0800, 6, 1234, 81, false}, {0x0800, 1, 0, 80, false}, {0x86dd, 6, 0, 0, true}, {0x8100, 6, 0, 0, true}, {0x0806, 0, 0, 0, false}} {
		b := make([]byte, 100)
		binary.BigEndian.PutUint16(b[12:14], tc.ether)
		b[14] = 0x45
		b[23] = tc.proto
		binary.BigEndian.PutUint16(b[34:36], tc.src)
		binary.BigEndian.PutUint16(b[36:38], tc.dst)
		n, e := vm.Run(b)
		if e != nil || (n > 0) != tc.want {
			t.Fatal(tc, n, e)
		}
	}
}
