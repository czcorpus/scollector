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

package storage

import (
	"encoding/binary"
	"strings"
)

const (
	LemmaToIDPrefix    byte = 0x00 // "lemma" -> tokenID
	SingleTokenPrefix  byte = 0x01 // tokenID -> frequency
	PairTokenPrefix    byte = 0x02 // (tokenID1, tokenID2) -> frequency
	IDToLemmaPrefix    byte = 0x03 // tokenID -> "lemma" (reverse lookup)
	PoSLemmaToIDPrefix byte = 0x04 // "lemma + PoS" -> tokenID
	IDToPoSLemmaPrefix byte = 0x05 // tokenID -> "lemma + PoS" (reverse lookup)
)

// encodeLemmaKey creates a byte key representation for Lemma -> Lemma ID entries
func encodeLemmaKey(lemma string) []byte {
	tmp := strings.Split(lemma, " ")
	lemmaBytes := []byte(lemma)
	key := make([]byte, 1+len(lemmaBytes))
	if len(tmp) == 1 {
		key[0] = LemmaToIDPrefix

	} else {
		key[0] = PoSLemmaToIDPrefix
	}
	copy(key[1:], lemmaBytes)
	return key
}

func decodeFrequency(data []byte) uint32 {
	return binary.LittleEndian.Uint32(data)
}

func decodeFrequencyAndDist(data []byte) (uint32, uint16) {
	return binary.LittleEndian.Uint32(data[:4]), binary.LittleEndian.Uint16(data[4:])
}

func mutualPositionToInt(v uint16) int {
	return 32768 - int(v)
}

func mutualPositionToUint16(v int) uint16 {
	if v > 16384 {
		panic("cannot encode position - distance overflow")
	}
	return uint16(32768 + v)
}

func encodeFrequency(freq uint32) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, freq)
	return buf
}

func encodeFrequencyAndDist(freq uint32, dist uint16) []byte {
	buf := make([]byte, 6)
	binary.LittleEndian.PutUint32(buf[:4], freq)
	binary.LittleEndian.PutUint16(buf[4:], dist)
	return buf
}

func encodePairTokenKey(token1ID, token2ID uint32) []byte {
	key := make([]byte, 9)
	key[0] = PairTokenPrefix
	binary.LittleEndian.PutUint32(key[1:5], token1ID)
	binary.LittleEndian.PutUint32(key[5:9], token2ID)
	return key
}

func tokenIDToKey(tokenID uint32) []byte {
	key := make([]byte, 5)
	key[0] = SingleTokenPrefix
	binary.LittleEndian.PutUint32(key[1:5], tokenID)
	return key
}

func tokenIDToValue(tokenID uint32) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, tokenID)
	return buf
}

// tokenIDToRIKey creates a key entry for the reverse index
func tokenIDToRIKey(tokenID uint32) []byte {
	key := make([]byte, 5)
	key[0] = IDToLemmaPrefix
	binary.LittleEndian.PutUint32(key[1:5], tokenID)
	return key
}
