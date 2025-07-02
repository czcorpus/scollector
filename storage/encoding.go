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
)

const (
	LemmaToIDPrefix   byte = 0x00 // "lemma" -> tokenID
	SingleTokenPrefix byte = 0x01 // tokenID -> frequency
	PairTokenPrefix   byte = 0x02 // (tokenID1, tokenID2) -> frequency
	IDToLemmaPrefix   byte = 0x03 // tokenID -> "lemma" (reverse lookup)
)

// encodeLemmaKey creates a byte key representation for Lemma -> Lemma ID entries
func encodeLemmaKey(lemma string) []byte {
	lemmaBytes := []byte(lemma)
	key := make([]byte, 1+len(lemmaBytes))
	key[0] = LemmaToIDPrefix
	copy(key[1:], lemmaBytes)
	return key
}

func decodeFrequency(data []byte) uint32 {
	return binary.LittleEndian.Uint32(data)
}

func encodeFrequency(freq uint32) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, freq)
	return buf
}

func encodePairTokenKey(token1ID, token2ID uint32) []byte {
	key := make([]byte, 9)
	key[0] = PairTokenPrefix
	binary.LittleEndian.PutUint32(key[1:5], token1ID)
	binary.LittleEndian.PutUint32(key[5:9], token2ID)
	return key
}

func encodeSingleTokenKey(tokenID uint32) []byte {
	key := make([]byte, 5)
	key[0] = SingleTokenPrefix
	binary.LittleEndian.PutUint32(key[1:5], tokenID)
	return key
}

func encodeTokenID(tokenID uint32) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, tokenID)
	return buf
}

func encodeIDToLemmaKey(tokenID uint32) []byte {
	key := make([]byte, 5)
	key[0] = IDToLemmaPrefix
	binary.LittleEndian.PutUint32(key[1:5], tokenID)
	return key
}
