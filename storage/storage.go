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

	"github.com/dgraph-io/badger"
)

func GetLemmaID(db *badger.DB, lemma string) (uint32, error) {
	var tokenID uint32
	err := db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(EncodeLemmaKey(lemma))
		if err != nil {
			return err
		}

		tokenIDBytes, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}

		tokenID = binary.LittleEndian.Uint32(tokenIDBytes)
		return nil
	})
	return tokenID, err
}
