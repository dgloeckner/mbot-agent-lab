// Package cyberpi uploads a MicroPython program to a Makeblock CyberPi (mBot2) over its USB
// serial link, the way mBlock does: F3/F4 frames, upload mode, file transfer in 80-byte blocks.
//
// The wire protocol was reverse-engineered by the projects below; this is a Go port of their
// protocol and upload sequence. Their MIT notices:
//
// PyOpenBlock (https://github.com/hrksgt/PyOpenBlock), MIT License
// Copyright (c) 2026 hrksgt
// Copyright (c) 2026 TWO PLUS THREE LIMITED
//
// mbot2-ble (https://github.com/Coletronix/mbot2-ble), MIT License
// Copyright (c) 2026 Coletronix
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.
package cyberpi

import (
	"bytes"
	"encoding/binary"
)

// Frame: F3, hdr, len_lo, len_hi, body..., sum(body)&0xFF, F4 with hdr = (F3+len_lo+len_hi)&0xFF.
const (
	head = 0xF3
	tail = 0xF4

	// maxBody bounds incoming frames; longer "frames" are program output that happens to
	// contain 0xF3.
	maxBody = 4096

	// BlockSize is the file-transfer chunk size mBlock uses.
	BlockSize = 80

	// StagingPath is where the file goes; the firmware moves /flash/_xx_main.py to
	// /flash/main.py when it restarts, so an interrupted upload keeps the old program.
	StagingPath = "/flash/_xx_main.py"
)

// Frame wraps body in an F3/F4 frame.
func Frame(body []byte) []byte {
	if len(body) > 0xFFFF {
		panic("cyberpi: frame body too large")
	}
	lo, hi := byte(len(body)), byte(len(body)>>8)
	out := make([]byte, 0, len(body)+6)
	out = append(out, head, head+lo+hi, lo, hi)
	out = append(out, body...)
	return append(out, sum(body), tail)
}

func sum(b []byte) byte {
	var s byte
	for _, c := range b {
		s += c
	}
	return s
}

// Ping switches the CyberPi into upload mode; it echoes the body.
var Ping = Frame([]byte{0x0D, 0x00, 0x00})

// ScriptFrame runs Python source on the CyberPi (type 0x28, mode 0x04 = exec, index 0).
func ScriptFrame(code string) []byte {
	body := []byte{0x28, 0x04, 0x00, 0x00}
	body = binary.LittleEndian.AppendUint16(body, uint16(len(code)))
	return Frame(append(body, code...))
}

// The two scripts mBlock sends around every upload: load the firmware's config module and turn
// the REPL off, so the uploaded program owns the serial port after the restart.
var (
	ScriptImportConfig = ScriptFrame("try:\n    import config\nexcept:\n    pass")
	ScriptReplOff      = ScriptFrame("try:\n    config.write_config(\"repl_enable\", False)\nexcept:\n    pass")
)

// FileChecksum is the XOR of the little-endian 32-bit words of data, zero padded.
func FileChecksum(data []byte) uint32 {
	var x uint32
	for i := 0; i < len(data); i += 4 {
		var w [4]byte
		copy(w[:], data[i:])
		x ^= binary.LittleEndian.Uint32(w[:])
	}
	return x
}

func fileFrame(cmd byte, payload []byte) []byte {
	body := []byte{0x01, 0x00, 0x5E, cmd}
	body = binary.LittleEndian.AppendUint16(body, uint16(len(payload)))
	return Frame(append(body, payload...))
}

// FileHeaderFrame announces size, checksum and path of the file to come (command 01).
func FileHeaderFrame(data []byte) []byte {
	p := []byte{0x00}
	p = binary.LittleEndian.AppendUint32(p, uint32(len(data)))
	p = binary.LittleEndian.AppendUint32(p, FileChecksum(data))
	return fileFrame(0x01, append(p, StagingPath...))
}

// FileBlockFrame carries one chunk of the file at its byte offset (command 02).
func FileBlockFrame(offset int, chunk []byte) []byte {
	p := binary.LittleEndian.AppendUint32(nil, uint32(offset))
	return fileFrame(0x02, append(p, chunk...))
}

// PadForWire appends newlines so len%80 == 4: the block layout mBlock produces and that is
// proven on hardware.
func PadForWire(src []byte) []byte {
	n := (4 - len(src)%BlockSize + BlockSize) % BlockSize
	out := make([]byte, len(src), len(src)+n)
	copy(out, src)
	return append(out, bytes.Repeat([]byte{'\n'}, n)...)
}

// Block is one file-transfer chunk.
type Block struct {
	Offset int
	Data   []byte
}

// Blocks splits data like mBlock: [0, len-4) in 80-byte blocks, then the last 4 bytes alone.
func Blocks(data []byte) []Block {
	if len(data) <= 4 {
		return []Block{{0, data}}
	}
	end := len(data) - 4
	var out []Block
	for off := 0; off < end; off += BlockSize {
		out = append(out, Block{off, data[off:min(off+BlockSize, end)]})
	}
	return append(out, Block{end, data[end:]})
}

// Reader splits the incoming byte stream into valid frames and everything else, which is
// program output (print) and goes to Text.
type Reader struct {
	Text func([]byte)
	buf  []byte
}

// Push adds bytes and returns the bodies of the complete, valid frames in arrival order.
// Incomplete frames stay buffered for the next call.
func (r *Reader) Push(chunk []byte) [][]byte {
	r.buf = append(r.buf, chunk...)
	var frames [][]byte
	for len(r.buf) > 0 {
		start := bytes.IndexByte(r.buf, head)
		if start < 0 {
			r.text(r.buf)
			r.buf = r.buf[:0]
			break
		}
		if start > 0 {
			r.text(r.buf[:start])
			r.buf = r.buf[start:]
		}
		if len(r.buf) < 4 {
			break
		}
		lo, hi := r.buf[2], r.buf[3]
		n := int(lo) | int(hi)<<8
		if r.buf[1] != head+lo+hi || n > maxBody {
			r.text(r.buf[:1])
			r.buf = r.buf[1:]
			continue
		}
		if len(r.buf) < n+6 {
			break
		}
		body := r.buf[4 : 4+n]
		if r.buf[4+n] != sum(body) || r.buf[5+n] != tail {
			r.text(r.buf[:1])
			r.buf = r.buf[1:]
			continue
		}
		frames = append(frames, bytes.Clone(body))
		r.buf = r.buf[n+6:]
	}
	// Keep the buffer from pinning a large backing array forever.
	r.buf = append([]byte(nil), r.buf...)
	return frames
}

func (r *Reader) text(b []byte) {
	if len(b) > 0 && r.Text != nil {
		r.Text(bytes.Clone(b))
	}
}

// Ack matchers, as lenient as the reference implementations.

// IsPingReply matches the echo of Ping.
func IsPingReply(body []byte) bool { return len(body) == 3 && body[0] == 0x0D }

// IsScriptReply matches the {"ret": ...} reply to a script frame.
func IsScriptReply(body []byte) bool {
	return bytes.Contains(body, []byte(`"ret"`)) || len(body) == 18
}

// IsFileAck matches the acknowledgement of a file header or block.
func IsFileAck(body []byte) bool {
	return bytes.Contains(body, []byte{0x5E, 0xF0}) || len(body) == 7
}
