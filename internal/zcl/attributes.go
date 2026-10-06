package zcl

import (
	"encoding/binary"
	"math"
)

// AttributeRecord represents a decoded attribute value from a ReportAttributes or ReadAttributesResponse.
type AttributeRecord struct {
	AttributeID uint16
	DataType    uint8
	Value       interface{}
}

// ParseAttributeReport extracts one or more attribute records from a ZCL report/read response payload.
func ParseAttributeReport(payload []byte) ([]AttributeRecord, error) {
	var records []AttributeRecord
	offset := 0

	for offset+3 <= len(payload) {
		attrID := binary.LittleEndian.Uint16(payload[offset : offset+2])
		dataType := payload[offset+2]
		offset += 3

		val, consumed, err := parseValue(dataType, payload[offset:])
		if err != nil {
			return records, err
		}
		offset += consumed

		records = append(records, AttributeRecord{
			AttributeID: attrID,
			DataType:    dataType,
			Value:       val,
		})
	}

	return records, nil
}

// ParseReadAttributesResponse extracts attribute records from a ZCL Read Attributes Response payload.
// Each record is formatted as:
// - Attribute identifier (2 octets)
// - Status (1 octet): 0x00 indicates SUCCESS
// - Attribute data type (1 octet, present only if Status == 0x00)
// - Attribute value (variable length, present only if Status == 0x00)
func ParseReadAttributesResponse(payload []byte) ([]AttributeRecord, error) {
	var records []AttributeRecord
	offset := 0

	for offset+3 <= len(payload) {
		attrID := binary.LittleEndian.Uint16(payload[offset : offset+2])
		status := payload[offset+2]
		offset += 3

		if status != 0x00 {
			continue
		}

		if offset >= len(payload) {
			break
		}

		dataType := payload[offset]
		offset++

		val, consumed, err := parseValue(dataType, payload[offset:])
		if err != nil {
			return records, err
		}
		offset += consumed

		records = append(records, AttributeRecord{
			AttributeID: attrID,
			DataType:    dataType,
			Value:       val,
		})
	}

	return records, nil
}

func parseValue(dataType uint8, data []byte) (interface{}, int, error) {
	if len(data) == 0 {
		return nil, 0, ErrFrameTooShort
	}

	switch dataType {
	case TypeBoolean:
		return data[0] != 0, 1, nil

	case TypeUint8, TypeData8, TypeBitmap8, TypeEnum8:
		return uint8(data[0]), 1, nil

	case TypeInt8:
		return int8(data[0]), 1, nil

	case TypeUint16, TypeData16, TypeBitmap16, TypeEnum16:
		if len(data) < 2 {
			return nil, 0, ErrFrameTooShort
		}
		return binary.LittleEndian.Uint16(data[0:2]), 2, nil

	case TypeInt16:
		if len(data) < 2 {
			return nil, 0, ErrFrameTooShort
		}
		return int16(binary.LittleEndian.Uint16(data[0:2])), 2, nil

	case TypeUint24:
		if len(data) < 3 {
			return nil, 0, ErrFrameTooShort
		}
		val := uint32(data[0]) | (uint32(data[1]) << 8) | (uint32(data[2]) << 16)
		return val, 3, nil

	case TypeUint32:
		if len(data) < 4 {
			return nil, 0, ErrFrameTooShort
		}
		return binary.LittleEndian.Uint32(data[0:4]), 4, nil

	case TypeUint48:
		if len(data) < 6 {
			return nil, 0, ErrFrameTooShort
		}
		val := uint64(data[0]) | (uint64(data[1]) << 8) | (uint64(data[2]) << 16) | (uint64(data[3]) << 24) | (uint64(data[4]) << 32) | (uint64(data[5]) << 40)
		return val, 6, nil

	case TypeInt32:
		if len(data) < 4 {
			return nil, 0, ErrFrameTooShort
		}
		return int32(binary.LittleEndian.Uint32(data[0:4])), 4, nil

	case TypeSinglePrec:
		if len(data) < 4 {
			return nil, 0, ErrFrameTooShort
		}
		bits := binary.LittleEndian.Uint32(data[0:4])
		return math.Float32frombits(bits), 4, nil

	case TypeCharStr, TypeOctetStr:
		length := int(data[0])
		if len(data) < 1+length {
			return nil, 0, ErrFrameTooShort
		}
		return string(data[1 : 1+length]), 1 + length, nil

	default:
		// Fallback: return raw byte if type length unknown
		return data[0], 1, nil
	}
}
