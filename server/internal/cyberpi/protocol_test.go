package cyberpi

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestFrameKnownBytes(t *testing.T) {
	cases := []struct {
		name string
		got  []byte
		want string
	}{
		{"ping", Ping, "f3 f6 03 00 0d 00 00 0d f4"},
		{"script x", ScriptFrame("x"), "f3 fa 07 00 28 04 00 00 01 00 78 a5 f4"},
		{"import config", ScriptImportConfig, "f3 20 2d 00 28 04 00 00 27 00" +
			hex.EncodeToString([]byte("try:\n    import config\nexcept:\n    pass")) + "b5 f4"},
		{"file header", FileHeaderFrame([]byte("ab\n\n")),
			"f3 14 21 00 01 00 5e 01 1b 00 00 04 00 00 00 61 62 0a 0a" +
				hex.EncodeToString([]byte(StagingPath)) + "2c f4"},
		{"file block", FileBlockFrame(80, []byte("xy")), "f3 ff 0c 00 01 00 5e 02 06 00 50 00 00 00 78 79 a8 f4"},
	}
	for _, c := range cases {
		if want := unhex(t, c.want); !bytes.Equal(c.got, want) {
			t.Errorf("%s:\n got % x\nwant % x", c.name, c.got, want)
		}
	}
}

func TestFrameLengthAboveByte(t *testing.T) {
	f := Frame(make([]byte, 300)) // lo 0x2c, hi 0x01
	if f[1] != (0xF3+0x2C+0x01)&0xFF || f[2] != 0x2C || f[3] != 0x01 || len(f) != 306 {
		t.Fatalf("header % x", f[:4])
	}
}

func TestFileChecksum(t *testing.T) {
	cases := []struct {
		in   string
		want uint32
	}{
		{"", 0},
		{"abcd", 0x64636261},
		{"abcdef", 0x64636261 ^ 0x00006665}, // last word zero padded
		{"abcdabcd", 0},
	}
	for _, c := range cases {
		if got := FileChecksum([]byte(c.in)); got != c.want {
			t.Errorf("FileChecksum(%q) = %#x, want %#x", c.in, got, c.want)
		}
	}
}

func TestPadForWire(t *testing.T) {
	for in, want := range map[int]int{0: 4, 3: 4, 4: 4, 5: 84, 84: 84, 85: 164, 163: 164, 164: 164} {
		src := bytes.Repeat([]byte{'a'}, in)
		got := PadForWire(src)
		if len(got) != want {
			t.Errorf("PadForWire(len %d) has len %d, want %d", in, len(got), want)
		}
		if !bytes.Equal(got[:in], src) || len(bytes.Trim(got[in:], "\n")) != 0 {
			t.Errorf("PadForWire(len %d) changed the source or padded with other bytes", in)
		}
	}
	src := []byte("abc")
	PadForWire(src)
	if string(src) != "abc" {
		t.Fatal("PadForWire modified its input")
	}
}

func TestBlocks(t *testing.T) {
	data := make([]byte, 164)
	for i := range data {
		data[i] = byte(i)
	}
	got := Blocks(data)
	want := []Block{{0, data[0:80]}, {80, data[80:160]}, {160, data[160:164]}}
	if len(got) != len(want) {
		t.Fatalf("%d blocks, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Offset != want[i].Offset || !bytes.Equal(got[i].Data, want[i].Data) {
			t.Errorf("block %d: offset %d len %d, want offset %d len %d",
				i, got[i].Offset, len(got[i].Data), want[i].Offset, len(want[i].Data))
		}
	}
	if b := Blocks([]byte("ab\n\n")); len(b) != 1 || b[0].Offset != 0 || len(b[0].Data) != 4 {
		t.Errorf("4-byte file: %+v", b)
	}
	if b := Blocks(make([]byte, 84)); len(b) != 2 || len(b[0].Data) != 80 || b[1].Offset != 80 || len(b[1].Data) != 4 {
		t.Errorf("84-byte file: %+v", b)
	}
}

func TestReaderSplitsFramesAndText(t *testing.T) {
	var text bytes.Buffer
	r := &Reader{Text: func(b []byte) { text.Write(b) }}
	ack := Frame([]byte{0x0D, 0x00, 0x00})
	bad := Frame([]byte{1, 2, 3})
	bad[len(bad)-2]++ // wrong checksum: text, not a frame
	stream := append([]byte("hello\n"), ack...)
	stream = append(stream, bad...)
	stream = append(stream, "bye"...)
	stream = append(stream, ack...)

	// Feed byte by byte: frames split across reads must still be found.
	var frames [][]byte
	for _, c := range stream {
		frames = append(frames, r.Push([]byte{c})...)
	}
	if len(frames) != 2 || !IsPingReply(frames[0]) || !IsPingReply(frames[1]) {
		t.Fatalf("frames = % x", frames)
	}
	if want := "hello\n" + string(bad) + "bye"; text.String() != want {
		t.Errorf("text = %q, want %q", text.String(), want)
	}
}

func TestReaderRejectsOversizedLength(t *testing.T) {
	var text bytes.Buffer
	r := &Reader{Text: func(b []byte) { text.Write(b) }}
	// 0xF3 followed by a valid-looking header claiming 0x2000 bytes: treated as text.
	in := []byte{0xF3, (0xF3 + 0x20) & 0xFF, 0x00, 0x20, 'o', 'k'}
	if f := r.Push(in); len(f) != 0 {
		t.Fatalf("frames = % x", f)
	}
	if !bytes.Equal(text.Bytes(), in) {
		t.Errorf("text = % x", text.Bytes())
	}
}

func TestAckMatchers(t *testing.T) {
	cases := []struct {
		name  string
		match func([]byte) bool
		body  []byte
		want  bool
	}{
		{"ping echo", IsPingReply, []byte{0x0D, 0x00, 0x00}, true},
		{"ping wrong type", IsPingReply, []byte{0x0E, 0x00, 0x00}, false},
		{"ping wrong len", IsPingReply, []byte{0x0D, 0x00}, false},
		{"script ret", IsScriptReply, append([]byte{0x28, 0x04, 0, 0, 12, 0}, `{"ret":None}`...), true},
		{"script len 18", IsScriptReply, make([]byte, 18), true},
		{"script other", IsScriptReply, []byte{0x28, 0x04, 0, 0, 0, 0}, false},
		{"file ack 5e f0", IsFileAck, []byte{0x01, 0x00, 0x5E, 0xF0, 0x02, 0x00, 0x00, 0x00}, true},
		{"file ack len 7", IsFileAck, make([]byte, 7), true},
		{"file other", IsFileAck, []byte{0x01, 0x00, 0x5E, 0x01}, false},
	}
	for _, c := range cases {
		if got := c.match(c.body); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
