package injection

import (
	"errors"
)

// ProtobufUtils handling for specific binary manipulation
type ProtobufUtils struct{}

func EncodeVarint(value uint64) []byte {
	buf := make([]byte, 0, 10) // Max varint size for 64-bit is 10 bytes
	for value >= 128 {
		buf = append(buf, byte((value&0x7F)|0x80))
		value >>= 7
	}
	buf = append(buf, byte(value))
	return buf
}

func ReadVarint(data []byte, offset int) (uint64, int, error) {
	var result uint64
	var shift uint64
	pos := offset

	for pos < len(data) {
		b := data[pos]
		result |= uint64(b&0x7F) << shift
		pos++
		if (b & 0x80) == 0 {
			return result, pos, nil
		}
		shift += 7
		if shift >= 64 {
			return 0, 0, errors.New("varint overflow")
		}
	}
	return 0, 0, errors.New("incomplete varint data")
}

// SkipField returns the new offset after skipping the field at the given offset
func SkipField(data []byte, offset int, wireType int) (int, error) {
	switch wireType {
	case 0: // Varint
		_, nextOffset, err := ReadVarint(data, offset)
		return nextOffset, err
	case 1: // 64-bit
		return offset + 8, nil
	case 2: // Length-delimited
		length, nextOffset, err := ReadVarint(data, offset)
		if err != nil {
			return 0, err
		}
		return nextOffset + int(length), nil
	case 5: // 32-bit
		return offset + 4, nil
	default:
		return 0, errors.New("unknown wire type")
	}
}

func RemoveField(data []byte, fieldNum int) ([]byte, error) {
	var result []byte
	offset := 0

	for offset < len(data) {
		startOffset := offset
		tag, nextOffset, err := ReadVarint(data, offset)
		if err != nil {
			return nil, err
		}
		
		wireType := int(tag & 7)
		currentField := int(tag >> 3)

		if currentField == fieldNum {
			// Skip
			nextOffset, err := SkipField(data, nextOffset, wireType)
			if err != nil {
				return nil, err
			}
			offset = nextOffset
		} else {
			// Copy
			endOffset, err := SkipField(data, nextOffset, wireType)
			if err != nil {
				return nil, err
			}
			result = append(result, data[startOffset:endOffset]...)
			offset = endOffset
		}
	}
	return result, nil
}

func CreateStringField(fieldNum int, value string) []byte {
	tag := (uint64(fieldNum) << 3) | 2
	bytes := []byte(value)
	
	tagBytes := EncodeVarint(tag)
	lenBytes := EncodeVarint(uint64(len(bytes)))
	
	result := make([]byte, 0, len(tagBytes)+len(lenBytes)+len(bytes))
	result = append(result, tagBytes...)
	result = append(result, lenBytes...)
	result = append(result, bytes...)
	return result
}

func CreateTimestampField(fieldNum int, seconds int64) []byte {
	// Timestamp message format: Field 1 (seconds) as varint
	innerTag := (uint64(1) << 3) | 0
	innerTagBytes := EncodeVarint(innerTag)
	secondsBytes := EncodeVarint(uint64(seconds))
	
	innerMsg := make([]byte, 0, len(innerTagBytes)+len(secondsBytes))
	innerMsg = append(innerMsg, innerTagBytes...)
	innerMsg = append(innerMsg, secondsBytes...)
	
	// Wrap in length delimited
	tag := (uint64(fieldNum) << 3) | 2
	tagBytes := EncodeVarint(tag)
	lenBytes := EncodeVarint(uint64(len(innerMsg)))
	
	result := make([]byte, 0, len(tagBytes)+len(lenBytes)+len(innerMsg))
	result = append(result, tagBytes...)
	result = append(result, lenBytes...)
	result = append(result, innerMsg...)
	return result
}

func CreateOAuthTokenInfo(accessToken string, refreshToken string, expiry int64) []byte {
	f1 := CreateStringField(1, accessToken)
	f2 := CreateStringField(2, "Bearer")
	f3 := CreateStringField(3, refreshToken)
	f4 := CreateTimestampField(4, expiry)
	
	combined := make([]byte, 0, len(f1)+len(f2)+len(f3)+len(f4))
	combined = append(combined, f1...)
	combined = append(combined, f2...)
	combined = append(combined, f3...)
	combined = append(combined, f4...)
	
	// Wrap as Field 6
	tag6 := (uint64(6) << 3) | 2
	tag6Bytes := EncodeVarint(tag6)
	lenBytes := EncodeVarint(uint64(len(combined)))
	
	result := make([]byte, 0, len(tag6Bytes)+len(lenBytes)+len(combined))
	result = append(result, tag6Bytes...)
	result = append(result, lenBytes...)
	result = append(result, combined...)
	
	return result
}

func GetField(data []byte, fieldNum int) []byte {
	offset := 0
	for offset < len(data) {
		tag, nextOffset, err := ReadVarint(data, offset)
		if err != nil {
			return nil
		}
		
		wireType := int(tag & 7)
		currentField := int(tag >> 3)

		if currentField == fieldNum {
			if wireType == 2 {
				length, start, err := ReadVarint(data, nextOffset)
				if err != nil {
					return nil
				}
				end := start + int(length)
				if end > len(data) {
					return nil
				}
				return data[start:end]
			}
			return nil
		}
		
		nextOffset, _ = SkipField(data, nextOffset, wireType)
		offset = nextOffset
	}
	return nil
}

func ExtractOAuthTokenInfo(data []byte) (string, string, error) {
	// Field 6 is OAuthTokenInfo
	field6 := GetField(data, 6)
	if field6 == nil {
		return "", "", errors.New("field 6 (OAuthTokenInfo) not found")
	}

	// Inside Field 6:
	// Field 1: Access Token
	// Field 3: Refresh Token
	
	accessTokenBytes := GetField(field6, 1)
	refreshTokenBytes := GetField(field6, 3)

	if accessTokenBytes != nil && refreshTokenBytes != nil {
		return string(accessTokenBytes), string(refreshTokenBytes), nil
	}
	return "", "", errors.New("incomplete token info")
}
