package capture

import "golang.org/x/net/bpf"

// Ethernet IPv4 TCP/UDP service-port filter. IPv6 and VLAN frames pass through
// for full userspace decoding (including extension headers); fragments pass for
// explicit unsupported accounting. No heuristic can drop legitimate payload.
func ServiceFilter(ports []int) ([]bpf.RawInstruction, error) {
	if len(ports) == 0 {
		return bpf.Assemble([]bpf.Instruction{bpf.RetConstant{Val: 0}})
	}
	code := []bpf.Instruction{
		bpf.LoadAbsolute{Off: 12, Size: 2},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 0x0800, SkipTrue: 7},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 0x86dd, SkipFalse: 1}, bpf.RetConstant{Val: 65535},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 0x8100, SkipFalse: 1}, bpf.RetConstant{Val: 65535},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 0x88a8, SkipFalse: 1}, bpf.RetConstant{Val: 65535}, bpf.RetConstant{Val: 0},
		bpf.LoadAbsolute{Off: 23, Size: 1},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 6, SkipTrue: 2},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 17, SkipTrue: 1}, bpf.RetConstant{Val: 0},
		bpf.LoadAbsolute{Off: 20, Size: 2}, bpf.JumpIf{Cond: bpf.JumpBitsSet, Val: 0x3fff, SkipFalse: 1}, bpf.RetConstant{Val: 65535},
	}
	if len(ports) > 900 {
		return bpf.Assemble(append(code, bpf.RetConstant{Val: 65535}))
	}
	code = append(code, bpf.LoadMemShift{Off: 14})
	for _, off := range []uint32{14, 16} {
		code = append(code, bpf.LoadIndirect{Off: off, Size: 2})
		for _, port := range ports {
			code = append(code, bpf.JumpIf{Cond: bpf.JumpEqual, Val: uint32(port), SkipFalse: 1}, bpf.RetConstant{Val: 65535})
		}
	}
	code = append(code, bpf.RetConstant{Val: 0})
	return bpf.Assemble(code)
}
