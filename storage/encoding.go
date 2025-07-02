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
)

// EncodeLemmaKey creates a byte key representation for Lemma -> Lemma ID entries
func EncodeLemmaKey(lemma string) []byte {
	lemmaBytes := []byte(lemma)
	key := make([]byte, 1+len(lemmaBytes))
	key[0] = LemmaToIDPrefix
	copy(key[1:], lemmaBytes)
	return key
}

func EncodeTokenID(tokenID uint32) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, tokenID)
	return buf
}
