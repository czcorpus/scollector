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
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/dgraph-io/badger/v4"
)

const (
	sortByLogDice SortingMeasure = "ldice"
	sortByTScore  SortingMeasure = "tscore"
)

type SortingMeasure string

func (m SortingMeasure) Validate() bool {
	return m == sortByLogDice || m == sortByTScore
}

// GetLemmaID returns numeric representation of a provided
// lemma. In case the lemma is not found, zero is returned
// (i.e. no error).
func (db *DB) GetLemmaID(lemma string) (uint32, error) {
	var tokenID uint32
	err := db.bdb.View(func(txn *badger.Txn) error {
		item, err := txn.Get(encodeLemmaKey(lemma))
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

type lemmaMatch struct {
	Value   string
	TokenID uint32
}

// GetLemmaIDsByPrefix returns all the
func (db *DB) GetLemmaIDsByPrefix(lemmaPrefix string) ([]lemmaMatch, error) {
	ans := make([]lemmaMatch, 0, 8)
	if !strings.Contains(lemmaPrefix, "_") {
		lemmaPrefix += "_"
	}
	err := db.bdb.View(func(txn *badger.Txn) error {
		key := encodeLemmaKey(lemmaPrefix)
		opts := badger.DefaultIteratorOptions
		opts.Prefix = key
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item().Key()[1:]
			var tokenID uint32
			err := it.Item().Value(func(val []byte) error {
				tokenID = binary.LittleEndian.Uint32(val)
				return nil
			})
			if err != nil {
				return err
			}
			ans = append(
				ans,
				lemmaMatch{
					Value:   strings.TrimSpace(string(item)),
					TokenID: tokenID,
				},
			)
		}
		return nil
	})
	return ans, err
}

func (db *DB) getLemmaByIDTxn(txn *badger.Txn, tokenID uint32) (string, error) {
	item, err := txn.Get(encodeIDToLemmaKey(tokenID))
	if err != nil {
		return "", err
	}

	lemmaBytes, err := item.ValueCopy(nil)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(lemmaBytes)), nil
}

func (db *DB) GetLemmaByID(tokenID uint32) (string, error) {
	var lemma string
	err := db.bdb.View(func(txn *badger.Txn) error {
		item, err := txn.Get(encodeIDToLemmaKey(tokenID))
		if err != nil {
			return err
		}

		lemmaBytes, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}

		lemma = strings.TrimSpace(string(lemmaBytes))
		return nil
	})
	return lemma, err
}

func getSingleTokenFreqCopy(txn *badger.Txn, tokenID uint32) (uint32, error) {
	key := encodeSingleTokenKey(tokenID)

	item, err := txn.Get(key)
	if err != nil {
		return 0, err
	}

	valBytes, err := item.ValueCopy(nil)
	if err != nil {
		return 0, err
	}

	if len(valBytes) != 4 {
		return 0, fmt.Errorf("invalid frequency data length: %d", len(valBytes))
	}

	return binary.LittleEndian.Uint32(valBytes), nil
}

func (db *DB) getSingleTokenFreq(tokenID uint32) (uint32, error) {
	var frequency uint32

	err := db.bdb.View(func(txn *badger.Txn) error {
		return db.getSingleTokenFreqTx(txn, tokenID, &frequency)
	})

	return frequency, err
}

// Version that works within an existing transaction
func (db *DB) getSingleTokenFreqTx(txn *badger.Txn, tokenID uint32, frequency *uint32) error {
	key := encodeSingleTokenKey(tokenID)

	item, err := txn.Get(key)
	if err != nil {
		return err
	}

	return item.Value(func(val []byte) error {
		if len(val) != 4 {
			return fmt.Errorf("invalid frequency data length: %d", len(val))
		}
		*frequency = binary.LittleEndian.Uint32(val)
		return nil
	})
}

func (db *DB) CalculateMeasures(lemma string, corpusSize int, limit int, sortBy SortingMeasure) ([]Collocation, error) {
	if limit < 0 {
		panic("CalculateMeasures - invalid limit value")
	}
	if corpusSize < 0 {
		panic("CalculateMeasures - invalid corpusSize value")
	}
	if !sortBy.Validate() {
		panic("CalculateMeasures - invalid sortBy value")
	}
	variants, err := db.GetLemmaIDsByPrefix(lemma)
	if err == badger.ErrKeyNotFound {
		return []Collocation{}, fmt.Errorf("failed to find matching lemma(s): %w", err)
	}

	var results []Collocation

	for _, lemmaMatch := range variants {
		// First, get F(x) - frequency of target lemma
		targetFreq, err := db.getSingleTokenFreq(lemmaMatch.TokenID)
		if err != nil {
			return nil, fmt.Errorf("failed to get target frequency: %w", err)
		}

		err = db.bdb.View(func(txn *badger.Txn) error {
			// Create prefix for all pairs starting with target lemma
			pairPrefix := make([]byte, 5)
			pairPrefix[0] = PairTokenPrefix
			binary.LittleEndian.PutUint32(pairPrefix[1:5], lemmaMatch.TokenID)

			opts := badger.DefaultIteratorOptions
			opts.Prefix = pairPrefix
			it := txn.NewIterator(opts)
			defer it.Close()

			for it.Rewind(); it.Valid(); it.Next() {
				item := it.Item()
				key := item.Key()

				// Extract second lemma ID from key
				secondLemmaID := binary.LittleEndian.Uint32(key[5:9])

				// Get F(x,y) - pair frequency
				var pairFreq uint32
				var pairDist uint16
				err := item.Value(func(val []byte) error {
					pairFreq, pairDist = decodeFrequencyAndDist(val)
					return nil
				})
				if err != nil {
					// TODO
					continue
				}

				// Get F(y) - frequency of second lemma
				secondFreq, err := getSingleTokenFreqCopy(txn, secondLemmaID)
				if err != nil {
					continue // Skip if we can't find single freq
				}
				logDice := 14.0 + math.Log2(float64(2*pairFreq)/float64(targetFreq+secondFreq))
				tscore := (float64(pairFreq) - float64(targetFreq)*float64(secondFreq)/float64(corpusSize)) / math.Sqrt(float64(pairFreq))
				secondLemma, err := db.getLemmaByIDTxn(txn, secondLemmaID)
				if err != nil {
					// TODO
					continue
				}

				results = append(results, Collocation{
					RawLemma:     lemmaMatch.Value,
					RawCollocate: secondLemma,
					LogDice:      logDice,
					TScore:       tscore,
					MutualDist:   mutualPositionToInt(pairDist),
				})
			}
			return nil
		})
	}
	switch sortBy {
	case sortByTScore:
		sort.Slice(results, func(i, j int) bool {
			return results[i].TScore > results[j].TScore
		})
	case sortByLogDice:
		sort.Slice(results, func(i, j int) bool {
			return results[i].LogDice > results[j].LogDice
		})
	}
	if len(results) > limit {
		results = results[:limit]
	}
	return results, err
}

func splitByLastUnderscore(s string) (string, string) {
	lastIndex := strings.LastIndex(s, "_")
	if lastIndex == -1 {
		return s, ""
	}
	return s[:lastIndex], s[lastIndex+1:]
}

// ------------------------------------

type Collocation struct {
	RawLemma     string
	RawCollocate string
	LogDice      float64
	TScore       float64
	MutualDist   int
}

func (res *Collocation) LemmaAndFn() (string, string) {
	return splitByLastUnderscore(res.RawLemma)
}

func (res *Collocation) CollocateAndFn() (string, string) {
	return splitByLastUnderscore(res.RawCollocate)
}

func (ldr Collocation) TabString() string {
	lemma1, deprel1 := splitByLastUnderscore(ldr.RawLemma)
	lemma2, deprel2 := splitByLastUnderscore(ldr.RawCollocate)
	return fmt.Sprintf("%s\t(%s)\t%s\t(%s)\t%01.2f\t%01.2f", lemma1, deprel1, lemma2, deprel2, ldr.LogDice, ldr.TScore)
}

// --------

func OpenDB(path string) (*DB, error) {
	opts := badger.DefaultOptions(path).
		WithValueLogFileSize(256 << 20). // 256MB value log files
		WithNumMemtables(8).             // More memtables for writes
		WithNumLevelZeroTables(8)

	ans := &DB{}
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open collocations database: %w", err)
	}
	ans.bdb = db
	return ans, nil
}
