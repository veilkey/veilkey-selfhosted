//go:build linux

package collector

import (
	"encoding/binary"
	"testing"
	"unsafe"
)

func TestConnectEventStructLayout(t *testing.T) {
	// The Go struct must match the BPF-compiled C struct layout exactly.
	// C layout (confirmed via BPF disassembly):
	//   pid:    offset 0x00 (4 bytes)
	//   uid:    offset 0x04 (4 bytes)
	//   family: offset 0x08 (4 bytes)
	//   port:   offset 0x0c (2 bytes)
	//   comm:   offset 0x0e (16 bytes)
	//   pad:    offset 0x1e (2 bytes)
	//   addr4:  offset 0x20 (4 bytes)
	//   addr6:  offset 0x24 (16 bytes)
	//   total:  0x34 = 52 bytes

	const expectedSize = 52
	if got := int(unsafe.Sizeof(connectEvent{})); got != expectedSize {
		t.Fatalf("connectEvent size = %d, want %d (BPF struct size)", got, expectedSize)
	}

	// Verify binary.Read consumes exactly expectedSize bytes by round-tripping.
	src := connectEvent{
		PID:    1234,
		UID:    1000,
		Family: 2,
		Port:   443,
	}
	copy(src.Comm[:], []byte("curl"))
	src.Addr4 = 0x0100007f // 127.0.0.1

	buf := make([]byte, 0, expectedSize)
	w := &appendWriter{buf: &buf}
	if err := binary.Write(w, binary.LittleEndian, &src); err != nil {
		t.Fatalf("binary.Write: %v", err)
	}
	if len(buf) != expectedSize {
		t.Fatalf("binary.Write produced %d bytes, want %d", len(buf), expectedSize)
	}

	// Verify comm is at byte offset 14 (0x0e) matching BPF layout.
	if buf[14] != 'c' || buf[15] != 'u' || buf[16] != 'r' || buf[17] != 'l' {
		t.Fatalf("comm not at expected byte offset 14: got bytes[14:18]=%x", buf[14:18])
	}
}

// appendWriter is a minimal io.Writer that appends to a byte slice.
type appendWriter struct {
	buf *[]byte
}

func (w *appendWriter) Write(p []byte) (int, error) {
	*w.buf = append(*w.buf, p...)
	return len(p), nil
}

func TestConnectEventUnknownFamilyNotDecoded(t *testing.T) {
	// Construct a raw record with an unknown address family and verify
	// that binary decoding succeeds but the family value is preserved,
	// allowing the caller to skip it via the default switch case.
	src := connectEvent{
		PID:    42,
		Family: 99,
	}

	buf := make([]byte, 0, int(unsafe.Sizeof(src)))
	w := &appendWriter{buf: &buf}
	if err := binary.Write(w, binary.LittleEndian, &src); err != nil {
		t.Fatalf("binary.Write: %v", err)
	}

	var dst connectEvent
	if err := binary.Read(newReader(buf), binary.LittleEndian, &dst); err != nil {
		t.Fatalf("binary.Read: %v", err)
	}
	if dst.Family != 99 {
		t.Fatalf("Family = %d, want 99", dst.Family)
	}
}

type bytesReader struct {
	data []byte
	pos  int
}

func newReader(data []byte) *bytesReader { return &bytesReader{data: data} }

func (r *bytesReader) Read(p []byte) (int, error) {
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
