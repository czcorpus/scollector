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
	"fmt"

	"github.com/dgraph-io/badger/v4"
)

type tokenIDSequence struct {
	value uint32
	cache map[string]uint32
}

func (tseq *tokenIDSequence) next(lemma string) uint32 {
	tseq.value++
	if tseq.value == 0 {
		panic("tokenIDSequence overflow")
	}
	tseq.cache[lemma] = tseq.value
	return tseq.value
}

func (tseq *tokenIDSequence) recall(lemma string) uint32 {
	// zero means = not found (we serve ids from 1)
	return tseq.cache[lemma]
}

func NewTokenIDSequence() *tokenIDSequence {
	return &tokenIDSequence{
		value: 0,
		cache: make(map[string]uint32),
	}
}

// --------------

func (db *DB) StoreSingleTokenFreqTx(txn *badger.Txn, tokenID uint32, frequency uint32) error {
	key := encodeSingleTokenKey(tokenID)
	value := encodeFrequency(frequency)
	return txn.Set(key, value)
}

func (db *DB) StorePairTokenFreqTx(txn *badger.Txn, token1ID, token2ID uint32, frequency uint32) error {
	key := encodePairTokenKey(token1ID, token2ID)
	value := encodeFrequency(frequency)
	return txn.Set(key, value)
}

func (db *DB) CreateTransaction() *badger.Txn {
	return db.bdb.NewTransaction(true)
}

func (db *DB) StoreLemmaTx(txn *badger.Txn, lemma string, tokenID uint32) error {
	key := encodeLemmaKey(lemma)
	value := encodeTokenID(tokenID)
	if err := txn.Set(key, value); err != nil {
		return err
	}
	// Store tokenID -> lemma mapping (reverse index)
	idKey := encodeIDToLemmaKey(tokenID)
	return txn.Set(idKey, []byte(lemma))
}

func (db *DB) StoreData(
	tidSeq *tokenIDSequence,
	singleFreqs map[string]int,
	pairFreqs map[[2]string]int,
	minPairFreq int) error {

	// use singleFreqs as source of lemmas and create indexes
	for lemma := range singleFreqs {

		err := db.bdb.Update(func(txn *badger.Txn) error {
			if err := db.StoreLemmaTx(txn, lemma, tidSeq.next(lemma)); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to store lemma: %w", err)
		}
	}

	// Process single token frequencies
	for lemma, lemmaFreq := range singleFreqs {
		err := db.bdb.Update(func(txn *badger.Txn) error {
			if err := db.StoreSingleTokenFreqTx(txn, tidSeq.recall(lemma), uint32(lemmaFreq)); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to store single freq: %w", err)
		}
	}

	// Process pair frequencies
	for lemmaPair, pairFreq := range pairFreqs {
		if pairFreq < minPairFreq {
			continue
		}
		err := db.bdb.Update(func(txn *badger.Txn) error {
			if err := db.StorePairTokenFreqTx(
				txn,
				tidSeq.recall(lemmaPair[0]),
				tidSeq.recall(lemmaPair[1]),
				uint32(pairFreq),
			); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to store pair freq: %w", err)
		}
	}

	return nil
}

// Convenience function to store or update frequency (incremental counting)
func (db *DB) IncrementSingleTokenFreq(tokenID uint32, increment uint32) error {
	return db.bdb.Update(func(txn *badger.Txn) error {
		// Try to get existing frequency
		var currentFreq uint32
		err := db.getSingleTokenFreqTx(txn, tokenID, &currentFreq)
		if err != nil && err != badger.ErrKeyNotFound {
			return err
		}

		// Add increment
		newFreq := currentFreq + increment

		// Store updated frequency
		key := encodeSingleTokenKey(tokenID)
		value := encodeFrequency(newFreq)
		return txn.Set(key, value)
	})
}

func (db *DB) IncrementPairTokenFreq(token1ID, token2ID uint32, increment uint32) error {
	return db.bdb.Update(func(txn *badger.Txn) error {
		key := encodePairTokenKey(token1ID, token2ID)

		// Try to get existing frequency
		var currentFreq uint32
		item, err := txn.Get(key)
		if err != nil && err != badger.ErrKeyNotFound {
			return err
		}
		if err == nil {
			err = item.Value(func(val []byte) error {
				currentFreq = decodeFrequency(val)
				return nil
			})
			if err != nil {
				return err
			}
		}

		// Add increment
		newFreq := currentFreq + increment

		// Store updated frequency
		value := encodeFrequency(newFreq)
		return txn.Set(key, value)
	})
}
