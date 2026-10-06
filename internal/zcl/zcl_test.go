package zcl

import (
	"bytes"
	"testing"
)

func TestBufferPool(t *testing.T) {
	pool := NewBufferPool(256)
	buf1 := pool.Get()
	if len(buf1) != 0 {
		t.Errorf("expected len 0, got %d", len(buf1))
	}
	if cap(buf1) < 256 {
		t.Errorf("expected cap >= 256, got %d", cap(buf1))
	}

	buf1 = append(buf1, 1, 2, 3)
	pool.Put(buf1)

	buf2 := pool.Get()
	if len(buf2) != 0 {
		t.Errorf("expected len 0 after reset, got %d", len(buf2))
	}
}

func TestZCLFrameEncodeDecode(t *testing.T) {
	orig := &Frame{
		Header: FrameControl{
			Type:                   FrameTypeClusterSpecific,
			ManufacturerSpecific:   false,
			Direction:              DirectionClientToServer,
			DisableDefaultResponse: true,
		},
		TransactionSequenceNum: 42,
		CommandID:              CmdOnOffOn,
		Payload:                []byte{0x01, 0x02, 0x03},
	}

	encoded, err := orig.Encode(nil)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if decoded.Header.Type != orig.Header.Type {
		t.Errorf("expected type %v, got %v", orig.Header.Type, decoded.Header.Type)
	}
	if decoded.TransactionSequenceNum != orig.TransactionSequenceNum {
		t.Errorf("expected seq %d, got %d", orig.TransactionSequenceNum, decoded.TransactionSequenceNum)
	}
	if decoded.CommandID != orig.CommandID {
		t.Errorf("expected cmd %d, got %d", orig.CommandID, decoded.CommandID)
	}
	if !bytes.Equal(decoded.Payload, orig.Payload) {
		t.Errorf("payload mismatch: expected %v, got %v", orig.Payload, decoded.Payload)
	}
}

func TestParseAttributeReport(t *testing.T) {
	// Attribute 0x0000 (OnOff), TypeBoolean (0x10), Value = 1 (true)
	// Attribute 0x0020 (Battery voltage), TypeUint8 (0x20), Value = 31 (3.1V)
	payload := []byte{
		0x00, 0x00, 0x10, 0x01,
		0x20, 0x00, 0x20, 0x1F,
	}

	records, err := ParseAttributeReport(payload)
	if err != nil {
		t.Fatalf("failed to parse attribute report: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	if records[0].AttributeID != 0x0000 || records[0].Value != true {
		t.Errorf("record 0 mismatch: %+v", records[0])
	}

	if records[1].AttributeID != 0x0020 || records[1].Value != uint8(31) {
		t.Errorf("record 1 mismatch: %+v", records[1])
	}
}

func TestParseReadAttributesResponse(t *testing.T) {
	// Attribute 0x0004 (Manufacturer): Status=0x00, TypeCharString (0x42), len=4, "Nous"
	// Attribute 0x0005 (Model): Status=0x00, TypeCharString (0x42), len=3, "A7Z"
	// Attribute 0x0006: Status=0x86 (Unsupported attribute, should be skipped)
	payload := []byte{
		0x04, 0x00, 0x00, 0x42, 0x04, 'N', 'o', 'u', 's',
		0x05, 0x00, 0x00, 0x42, 0x03, 'A', '7', 'Z',
		0x06, 0x00, 0x86,
	}

	records, err := ParseReadAttributesResponse(payload)
	if err != nil {
		t.Fatalf("failed to parse read attributes response: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	if records[0].AttributeID != 0x0004 || records[0].Value != "Nous" {
		t.Errorf("record 0 mismatch: %+v", records[0])
	}

	if records[1].AttributeID != 0x0005 || records[1].Value != "A7Z" {
		t.Errorf("record 1 mismatch: %+v", records[1])
	}
}
