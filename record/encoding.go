// Copyright 2025 Tomas Machalek <tomas.machalek@gmail.com>
// Copyright 2025 Department of Linguistics,
//                Faculty of Arts, Charles University
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package record

import (
	"encoding/binary"
	"fmt"
	"math"
)

const (
	metadataPrefix     byte = 0x01
	lemmaToIDPrefix    byte = 0x02 // "lemma" -> tokenID
	idToLemmaPrefix    byte = 0x03 // tokenID -> "lemma" (reverse lookup)
	singleTokenPrefix  byte = 0x04 // tokenID -> frequency
	pairTokenPrefix    byte = 0x05 // (tokenID1, tokenID2) -> frequency&dist (where tokenID1 is HEAD)
	revPairTokenPrefix byte = 0x06 // (tokenID1, tokenID2) -> frequency&dist (where tokenID1 is DEPENDENT)

	MetadataKeyImportProfile byte = 0x01
)

type DecodedKey struct {
	Token1ID uint32
	Pos1     byte
	Deprel   uint16
	Token2ID uint32
	Pos2     byte
	TextType byte
}

// EncodeLemmaKey creates a byte key representation for (Lemma) -> (Lemma ID) entries
// Note that this has nothing to do with other token properties like PoS or deprel.
// This just maps a string to a byte DB key so we can map tokens to numbers.
func EncodeLemmaKey(lemma TokenFreq) []byte {
	key := make([]byte, 1+len([]byte(lemma.Lemma)))
	key[0] = lemmaToIDPrefix
	copy(key[1:], []byte(lemma.Lemma))
	return key
}

// EncodeLemmaPrefixKey creates a byte key representation for (Lemma) -> (Lemma ID) entries
func EncodeLemmaPrefixKey(lemmaPrefix string) []byte {
	var lemmaBytes []byte
	lemmaBytes = []byte(lemmaPrefix)
	key := make([]byte, 1+len(lemmaBytes))
	key[0] = lemmaToIDPrefix
	copy(key[1:], lemmaBytes)
	return key
}

func CreateMetadataKey(keyID byte) []byte {
	return []byte{metadataPrefix, keyID}
}

// CollFreqKey produces a byte slice representing a DB entry with a collocation freq. info.
// The key is composed in a way allowing for searching via textType without knowing token1's deprel
// or even token2 properties (this is given by prefix key search)
// The key looks like this:
// byte 0:    key type
// byte 1-4:  token1 ID
// byte 5:    token1 PoS
// byte 6:    text type
// byte 7-8:  token1 deprel
// byte 9-12: token2 ID
// byte 13:   token2 PoS
// byte 14-15: token2 deprel
func CollFreqKey(t1IsHead bool, token1ID uint32, pos1, textType byte, deprel uint16, token2ID uint32, pos2 byte) []byte {
	key := make([]byte, 1+4+1+1+2+4+1)
	if t1IsHead {
		key[0] = pairTokenPrefix

	} else {
		key[0] = revPairTokenPrefix
	}
	binary.LittleEndian.PutUint32(key[1:5], token1ID)
	key[5] = pos1
	key[6] = textType
	binary.LittleEndian.PutUint16(key[7:9], deprel)
	binary.LittleEndian.PutUint32(key[9:13], token2ID)
	key[13] = pos2
	return key
}

// DecodeCollFreqKey is a reverse function to CollFreqKey. From a byte slice,
// it extracts all the collocation properties.
func DecodeCollFreqKey(key []byte) DecodedKey {
	return DecodedKey{
		Token1ID: binary.LittleEndian.Uint32(key[1:5]),
		Pos1:     key[5],
		TextType: key[6],
		Deprel:   binary.LittleEndian.Uint16(key[7:9]),
		Token2ID: binary.LittleEndian.Uint32(key[9:13]),
		Pos2:     key[13],
	}
}

// AllCollFreqsOfToken generates a db key to search for all
// the collocation freq. records of this token (where the token
// is the first one).
// The isHead argument decides which role the token plays:
// isHead == true: [tokenID] <-- deprel --- [SEARCHED_TOKEN]
// isHead == false: [tokenID] --- deprel --> [SEARCHED_TOKEN]
func AllCollFreqsOfToken(isHead bool, tokenID uint32) []byte {
	key := make([]byte, 5)
	if isHead {
		key[0] = pairTokenPrefix

	} else {
		key[0] = revPairTokenPrefix
	}
	binary.LittleEndian.PutUint32(key[1:5], tokenID)
	return key
}

// TokenFreqKey generates a key for searching of single token
// frequencies.
// Note that this is not for generating search prefix keys as this
// always produces full size keys even if you provide e.g. a zero 'pos'.
//
// For generating search keys, use TokenFreqSearchKey which generates
// proper key prefix in case you provide zero pos, textType or deprel.
func TokenFreqKey(tokenID uint32, pos, textType byte) []byte {
	key := make([]byte, 1+4+1+1)
	key[0] = singleTokenPrefix
	binary.LittleEndian.PutUint32(key[1:5], tokenID)
	key[5] = pos
	key[6] = textType
	return key
}

// TokenFreqSearchKey is a searching variant of TokenFreqKey. This function,
// produces byte slice key without trailing zero values with pos having the
// highest priority following by textType and deprel. I.e. if you provide zero
// pos, then the key will contain just token ID (and the key identifier zero byte).
func TokenFreqSearchKey(tokenID uint32, pos, textType byte) []byte {
	key := make([]byte, 5, 9)
	key[0] = singleTokenPrefix
	binary.LittleEndian.PutUint32(key[1:5], tokenID)
	if pos > 0 {
		key = append(key, pos)
		if textType > 0 {
			key = append(key, textType)
		}
	}
	return key
}

// DecodeTokenFreqKey is a reverse function to TokenFreqKey. Given the provided
// key, it extracts all the included properties. Note that the returned value
// type DecodedKey is the same as in case of the collocation freq. records.
// It means that here, all the attributes belonging to the second lemma will be
// always zero.
func DecodeTokenFreqKey(key []byte) DecodedKey {
	if len(key) < 5 {
		panic(fmt.Sprintf("DecodeTokenFreqKey failed, expected at least length of 5, found: %d", len(key)))
	}
	ans := DecodedKey{
		Token1ID: binary.LittleEndian.Uint32(key[1:5]),
	}
	if len(key) >= 6 {
		ans.Pos1 = key[5]
	}
	if len(key) >= 7 {
		ans.TextType = key[6]
	}
	if len(key) >= 8 {
		ans.Deprel = binary.LittleEndian.Uint16(key[7:9])
	}
	return ans
}

func TokenIDToBytes(tokenID uint32) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, tokenID)
	return buf
}

// TokenIDToRevIndexKey creates a key entry for the reverse index
func TokenIDToRevIndexKey(tokenID uint32) []byte {
	key := make([]byte, 5)
	key[0] = idToLemmaPrefix
	binary.LittleEndian.PutUint32(key[1:5], tokenID)
	return key
}

// EncodeDistance encodes a floating-point distance to a byte.
// Range: -12.7 to +12.7 with 0.1 precision
// Encoding: 0-127 for negative values (-12.7 to -0.1), 128-255 for positive values (0.0 to +12.7)
func EncodeDistance(distance float64) byte {
	// Scale by 10 for 0.1 precision
	scaled := int32(math.Round(distance * 10))

	if scaled < 0 {
		// Negative: map -127 to -1 -> 0 to 126
		if scaled < -127 {
			scaled = -127
		}
		return byte(-scaled - 1)
	} else {
		// Positive: map 0 to 127 -> 128 to 255
		if scaled > 127 {
			scaled = 127
		}
		return byte(scaled + 128)
	}
}

// DecodeDistance decodes a byte back to a floating-point distance.
func DecodeDistance(encoded byte) float64 {
	if encoded < 128 {
		// Negative value: 0-127 maps to -0.1 to -12.7
		return float64(-(int32(encoded) + 1)) / 10.0
	} else {
		// Positive value: 128-255 maps to 0.0 to +12.7
		return float64(int32(encoded)-128) / 10.0
	}
}

// CollocValue represents the binary format for collocation values
type CollocValue struct {
	Freq uint32
	Dist float64
}

// EncodeCollocValue encodes frequency and distance into a 5-byte binary format
func EncodeCollocValue(freq uint32, avgDist float64) []byte {
	value := make([]byte, 5)
	binary.LittleEndian.PutUint32(value[0:4], freq)
	value[4] = EncodeDistance(avgDist)
	return value
}

// DecodeCollocValue decodes a 5-byte binary format back to frequency and distance
func DecodeCollocValue(data []byte) CollocValue {
	if len(data) != 5 {
		panic(fmt.Sprintf("DecodeCollocValue expected 5 bytes, got %d", len(data)))
	}
	return CollocValue{
		Freq: binary.LittleEndian.Uint32(data[0:4]),
		Dist: DecodeDistance(data[4]),
	}
}

// TokenValue represents the binary format for token frequency values
type TokenValue struct {
	Freq uint32
}

// EncodeTokenValue encodes frequency into a 4-byte binary format
func EncodeTokenValue(freq uint32) []byte {
	value := make([]byte, 4)
	binary.LittleEndian.PutUint32(value, freq)
	return value
}

// DecodeTokenValue decodes a 4-byte binary format back to frequency
func DecodeTokenValue(data []byte) TokenValue {
	if len(data) != 4 {
		panic(fmt.Sprintf("DecodeTokenValue expected 4 bytes, got %d", len(data)))
	}
	return TokenValue{
		Freq: binary.LittleEndian.Uint32(data),
	}
}
