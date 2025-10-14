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
	"crypto/sha1"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/czcorpus/depreldb/record"
	"github.com/dgraph-io/badger/v4"
	"github.com/rs/zerolog/log"
)

const (
	sortByLogDice SortingMeasure = "ldice"
	sortByTScore  SortingMeasure = "tscore"
	sortByLMI     SortingMeasure = "lmi"
	sortByLL      SortingMeasure = "ll"
	sortByRRF     SortingMeasure = "rrf"
)

type SortingMeasure string

func (m SortingMeasure) Validate() bool {
	return m == sortByLogDice || m == sortByTScore || m == sortByLMI || m == sortByRRF
}

// -------

type itemsWalktrhoughCache struct {
	db                *DB
	idToLemmaCache    map[uint32]string
	rawTokenFreqCache map[string][]record.RawTokenFreq
}

func (clm *itemsWalktrhoughCache) getLemmaByIDTxn(txn *badger.Txn, tokenID uint32) (string, error) {
	if clm.idToLemmaCache == nil {
		clm.idToLemmaCache = make(map[uint32]string)
	}
	var err error
	ans, ok := clm.idToLemmaCache[tokenID]
	if !ok {
		ans, err = clm.db.getLemmaByIDTxn(txn, tokenID)
		if err != nil {
			return "", err
		}
		clm.idToLemmaCache[tokenID] = ans
	}
	return ans, nil
}

func (clm *itemsWalktrhoughCache) getRawTokenFreqTx(txn *badger.Txn, tokenID uint32, pos, textType byte) ([]record.RawTokenFreq, error) {
	if clm.rawTokenFreqCache == nil {
		clm.rawTokenFreqCache = make(map[string][]record.RawTokenFreq)
	}
	srchKey := record.TokenFreqSearchKey(tokenID, pos, textType)
	ans, ok := clm.rawTokenFreqCache[string(srchKey)]
	var err error
	if !ok {
		ans, err = clm.db.getRawTokenFreqTx(txn, tokenID, pos, textType)
		if err != nil {
			return []record.RawTokenFreq{}, err
		}
		clm.rawTokenFreqCache[string(srchKey)] = ans
	}
	return ans, nil
}

// -------

// GetLemmaID returns numeric representation of a provided
// lemma. In case the lemma is not found, zero is returned
// (i.e. no error).
func (db *DB) GetLemmaID(lemmaEntry record.TokenFreq) (uint32, error) {
	var tokenID uint32
	err := db.bdb.View(func(txn *badger.Txn) error {
		item, err := txn.Get(record.EncodeLemmaKey(lemmaEntry))
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

type lemmaWithID struct {
	Value   string
	TokenID uint32
}

// GetLemmaIDsByPrefix returns all the
func (db *DB) GetLemmaIDsByPrefix(lemmaPrefix string) ([]lemmaWithID, error) {
	ans := make([]lemmaWithID, 0, 8)
	err := db.bdb.View(func(txn *badger.Txn) error {
		key := record.EncodeLemmaPrefixKey(lemmaPrefix)
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
				lemmaWithID{
					Value:   strings.TrimSpace(string(item)),
					TokenID: tokenID,
				},
			)
		}
		return nil
	})
	return ans, err
}

type LemmaProps struct {
	Pos      string
	Deprel   string
	Freq     int
	TextType string
}

func (db *DB) GetMatchingLemmaProps(tokenID uint32) ([]LemmaProps, error) {
	var results []LemmaProps
	err := db.bdb.View(func(txn *badger.Txn) error {
		searchKey := record.TokenFreqSearchKey(tokenID, 0, 0)
		opts := badger.DefaultIteratorOptions
		opts.Prefix = searchKey
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			key := it.Item().Key()
			decodedKey := record.DecodeTokenFreqKey(key)
			pos := record.UDPosFromByte(decodedKey.Pos1)
			deprel := record.UDDeprelFromUint16(decodedKey.Deprel)
			textType := db.textTypes.RawToReadable(decodedKey.TextType)

			var tokenValue record.TokenValue
			err := it.Item().Value(func(val []byte) error {
				tokenValue = record.DecodeTokenValue(val)
				return nil
			})
			if err != nil {
				return err
			}

			results = append(results, LemmaProps{
				Pos:      pos.Readable,
				Deprel:   deprel.Readable,
				Freq:     int(tokenValue.Freq),
				TextType: textType,
			})
		}
		return nil
	})
	return results, err
}

func (db *DB) getLemmaByIDTxn(txn *badger.Txn, tokenID uint32) (string, error) {
	item, err := txn.Get(record.TokenIDToRevIndexKey(tokenID))
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
		item, err := txn.Get(record.TokenIDToRevIndexKey(tokenID))
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

func (db *DB) GetSingleTokenFreq(tokenID uint32, pos, textType byte) ([]record.TokenFreq, error) {
	ans := []record.TokenFreq{}
	err := db.bdb.View(func(txn *badger.Txn) error {
		tmp, err := db.getSingleTokenFreqTx(txn, tokenID, pos, textType)
		if err != nil {
			return err
		}
		ans = tmp
		return nil
	})

	return ans, err
}

func (db *DB) getSingleTokenFreqTx(txn *badger.Txn, tokenID uint32, pos, textType byte) ([]record.TokenFreq, error) {
	cachedTokIDs := itemsWalktrhoughCache{db: db}
	rawItems, err := db.getRawTokenFreqTx(txn, tokenID, pos, textType)
	if err != nil {
		return []record.TokenFreq{}, err
	}
	ans := make([]record.TokenFreq, len(rawItems))
	for i, ritem := range rawItems {
		var rec record.TokenFreq
		rec.Freq = int(ritem.Freq)
		lemma, err := cachedTokIDs.getLemmaByIDTxn(txn, ritem.TokenID)
		if err != nil {
			return []record.TokenFreq{}, err
		}
		rec.Lemma = lemma
		rec.TextType = record.TextType{
			Readable: db.textTypes.RawToReadable(ritem.TextType),
			Raw:      ritem.TextType,
		}
		rec.PoS = record.UDPosFromByte(ritem.PoS)
		ans[i] = rec
	}
	return ans, nil
}

// getRawTokenFreqTx searches for all tokens matching provided properties using
// a provided transaction.
// Attributes pos, textType, deprel are optional (can be set to zero) to search
// for more variants. But note that they are hierarchical - if pos is zero than
// other two are ignored. If pos is set and textType is zero, deprel is ignored.
//
// Also note that even if it is possible to filter by deprel, this value is not
// a part of the result list item type.
func (db *DB) getRawTokenFreqTx(txn *badger.Txn, tokenID uint32, pos, textType byte) ([]record.RawTokenFreq, error) {
	ans := make([]record.RawTokenFreq, 0, 100)
	srchKey := record.TokenFreqSearchKey(tokenID, pos, textType)
	opts := badger.DefaultIteratorOptions
	opts.Prefix = srchKey
	it := txn.NewIterator(opts)
	defer it.Close()

	for it.Rewind(); it.Valid(); it.Next() {
		var tokenValue record.TokenValue
		err := it.Item().Value(func(val []byte) error {
			tokenValue = record.DecodeTokenValue(val)
			return nil
		})
		if err != nil {
			return []record.RawTokenFreq{}, err
		}
		decKey := record.DecodeTokenFreqKey(it.Item().Key())
		ans = append(
			ans,
			record.RawTokenFreq{
				TokenID:  tokenID,
				Freq:     tokenValue.Freq,
				PoS:      decKey.Pos1,
				TextType: decKey.TextType,
			},
		)
	}
	return ans, nil
}

// ------

type DeprelVariant struct {
	Value string `json:"value"`
	Freq  int    `json:"freq"`
}

func (dv DeprelVariant) Key() string {
	return dv.Value
}

// ------

// GetLemmaDeprelValues searches for all the deprel variants providing
// the lemmaId is in role of a dependent token (i.e. not a "head").
// Found values are returned in the order starting with the most frequent
// items.
func (db *DB) GetLemmaDeprelValues(lemmaId uint32) ([]DeprelVariant, error) {
	counts := make(map[uint16]int)
	err := db.bdb.View(func(txn *badger.Txn) error {
		pairPrefix := record.AllCollFreqsOfToken(false, lemmaId)
		opts := badger.IteratorOptions{
			Prefix:         pairPrefix,
			PrefetchValues: true,
			PrefetchSize:   1000,
		}
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			key := item.Key()
			decKey := record.DecodeCollFreqKey(key)
			var collValue record.CollocValue
			err := item.Value(func(val []byte) error {
				collValue = record.DecodeCollocValue(val)
				return nil
			})
			if err != nil {
				return fmt.Errorf("failed to get colloc value from db: %w", err)
			}
			counts[decKey.Deprel] += int(collValue.Freq)
		}
		return nil
	})
	if err != nil {
		return []DeprelVariant{}, fmt.Errorf("failed to get lemma deprel values: %w", err)
	}
	ans := make([]DeprelVariant, len(counts))
	i := 0
	for k, v := range counts {
		ans[i] = DeprelVariant{Value: record.UDDeprelFromUint16(k).Readable, Freq: v}
		i++
	}
	slices.SortFunc(ans, func(v1, v2 DeprelVariant) int { return v2.Freq - v1.Freq })
	return ans, err
}

type SearchFilter func(pos1 byte, deprel uint16, pos2 byte, textType byte, dist float64) bool

// ------

// CalculateMeasures searches for all the matching collocates and calculates
// their Log-Dice and T-Score in collocations with the searched 'lemma'.
//
// note: for more convenient access, use scoll.Calculator
func (db *DB) CalculateMeasures(
	lemma, pos, textType string,
	lemmaIsPrefix bool,
	isHead *bool,
	maxAvgCollocateDist float64,
	limit int,
	sortBy SortingMeasure,
	collocateGroupByPos, groupByDeprel, collocateGroupByTextType bool,
	customFilter SearchFilter,
) ([]Collocation, error) {
	if limit < 0 {
		panic("CalculateMeasures - invalid limit value")
	}
	if !sortBy.Validate() {
		panic("CalculateMeasures - invalid sortBy value")
	}
	// first we find matching lemmas without considering other attributes
	// (PoS, deprel). If lemmaIsPrefix is false, then we should always find a single
	// token ID matching the result.
	variants, err := db.GetLemmaIDsByPrefix(lemma)
	if err == badger.ErrKeyNotFound {
		return []Collocation{}, fmt.Errorf("failed to find matching lemma(s): %w", err)
	}

	var results []Collocation
	ttID := db.textTypes.ReadableToRaw(textType)
	posID := record.UDPoSMapping[pos]
	sumFreqs1 := newTokenFreqGrouping()
	sumFreqs2 := newTokenFreqGrouping()
	sumCollFreqs := newCollFreqGrouping()

	// if user entered part of speech, we need to distinguish
	// the same lemmata with different pos in all the parts where
	// the searched lemma occurs
	if pos != "" {
		sumFreqs1.GroupByPos()
		sumCollFreqs.GroupByPos1()
	}

	// if user wanted a concrete text type, we need to "group by" it
	// in all the data (F(x), F(y), F(x, y)) so we will be able to remove
	// unwanted text types
	if textType != "" || collocateGroupByTextType {
		sumFreqs1.GroupByTT()
		sumFreqs2.GroupByTT()
		sumCollFreqs.GroupByTT()
	}

	// if groupByDeprel is true, it means, user wants separate occurrences
	// of different deprels for the same lemmas
	if groupByDeprel {
		sumCollFreqs.GroupByDeprel()
	}

	if collocateGroupByPos {
		sumFreqs2.GroupByPos()
		sumCollFreqs.GroupByPos2()
	}

	walkthruCache := itemsWalktrhoughCache{db: db}
	numProcVariants := 0
	t0 := time.Now()

	err = db.bdb.View(func(txn *badger.Txn) error {
		for _, lemmaMatch := range variants {
			if !lemmaIsPrefix && lemmaMatch.Value != lemma {
				continue
			}
			// First, get F(x) (i.e. freq. of the searched lemma). This search respects
			// possible provided PoS and text type specification. Attribute deprel cannot
			// be used in filter this way so it is filtered later (if needed).
			partialFreqs1, err := walkthruCache.getRawTokenFreqTx(txn, lemmaMatch.TokenID, posID, ttID)
			if err != nil {
				return fmt.Errorf("failed to calculate collocation scores: %w", err)
			}
			for _, pf1 := range partialFreqs1 {
				sumFreqs1.add(pf1)
			}

			var headDepSearches []bool
			if isHead == nil {
				headDepSearches = []bool{true, false}

			} else {
				headDepSearches = []bool{*isHead}
			}
			for _, directionFlag := range headDepSearches {
				pairPrefix := record.AllCollFreqsOfToken(directionFlag, lemmaMatch.TokenID)
				opts := badger.IteratorOptions{
					Prefix:         pairPrefix,
					PrefetchValues: true,
					PrefetchSize:   1000,
				}
				it := txn.NewIterator(opts)
				defer it.Close()
				numDbItems := 0

				for it.Rewind(); it.Valid(); it.Next() {
					item := it.Item()
					key := item.Key()
					decKey := record.DecodeCollFreqKey(key)

					if ttID > 0 && decKey.TextType != ttID {
						continue
					}

					var collValue record.CollocValue
					// Get F(x,y) frequency information
					err := item.Value(func(val []byte) error {
						collValue = record.DecodeCollocValue(val)
						return nil
					})
					if err != nil {
						// TODO
						fmt.Fprintf(os.Stderr, "failed to get freqs from db: %s", err)
						continue
					}

					if customFilter != nil && !customFilter(
						decKey.Pos1, decKey.Deprel, decKey.Pos2, decKey.TextType, collValue.Dist) {
						continue
					}

					if maxAvgCollocateDist > 0 && math.Abs(collValue.Dist) > maxAvgCollocateDist {
						continue
					}

					// F(x, y)
					sumCollFreqs.add(record.RawCollocFreq{
						Token1ID: decKey.Token1ID,
						PoS1:     decKey.Pos1,
						Deprel:   decKey.Deprel,
						Token2ID: decKey.Token2ID,
						PoS2:     decKey.Pos2,
						Freq:     collValue.Freq,
						AVGDist:  collValue.Dist,
						TextType: decKey.TextType,
					})

					// Get F(y) - frequency of second lemma
					partialSplitFreq2, err := walkthruCache.getRawTokenFreqTx(
						txn, decKey.Token2ID, decKey.Pos2, ttID)
					if err != nil {
						continue // Skip if we can't find single freq
					}
					for _, psf2 := range partialSplitFreq2 {
						sumFreqs2.add(psf2)
					}
					numDbItems++
				}
			}
			for _, val := range sumCollFreqs.Iter {
				lemma2, err := walkthruCache.getLemmaByIDTxn(txn, val.Token2ID)
				if err != nil {
					fmt.Fprintln(os.Stderr, "err: ", err)
					// TODO !!
				}
				f1 := sumFreqs1.get(val.GroupingKeyLemma1Binary())
				f2 := sumFreqs2.get(val.GroupingKeyLemma2Binary())

				logDice := 14.0 + math.Log2(float64(2*val.Freq)/float64(f1.Freq+f2.Freq))
				tscore := (float64(val.Freq) - (float64(f1.Freq)*float64(f2.Freq))/float64(db.Metadata.CorpusSize)) / math.Sqrt(float64(val.Freq))
				lmi := float64(val.Freq) * math.Log2(float64(db.Metadata.CorpusSize)*float64(val.Freq)/float64(f1.Freq*f2.Freq))
				ll := LLScore(val.Freq, f1.Freq, f2.Freq, db.Metadata.CorpusSize)
				results = append(results, Collocation{
					Lemma: CollMember{
						Value: lemmaMatch.Value,
						PoS:   pos,
					},
					Deprel: db.DeprelMapping.GetRev(val.Deprel),
					Collocate: CollMember{
						Value: lemma2,
						PoS:   record.UDPosFromByte(val.PoS2).Readable,
					},
					LogDice:       logDice,
					TScore:        tscore,
					LMI:           lmi,
					TextType:      db.textTypes.RawToReadable(val.TextType),
					LogLikelihood: ll,
					MutualDist:    val.AVGDist,
				})
				numProcVariants++
			}
		}

		return nil
	})
	if err != nil {
		return []Collocation{}, err
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
	case sortByLMI:
		sort.Slice(results, func(i, j int) bool {
			return results[i].LMI > results[j].LMI
		})
	case sortByLL:
		sort.Slice(results, func(i, j int) bool {
			return results[i].LogLikelihood > results[j].LogLikelihood
		})
	case sortByRRF:
		SortByRRF(results)
	}

	if len(results) > limit {
		results = results[:limit]
	}
	log.Debug().
		Int("numTried", numProcVariants).
		Str("procTime", fmt.Sprintf("%1.2f", time.Since(t0).Seconds())).
		Msg("finished collocation search")
	return results, err
}

// ------------------------------------

type CollMember struct {
	Value string `json:"value"`
	PoS   string `json:"pos"`
}

type roundedFloat float64

func (rf roundedFloat) MarshalJSON() ([]byte, error) {
	rounded := math.Round(float64(rf)*1000) / 1000
	return json.Marshal(rounded)
}

type Collocation struct {
	Lemma         CollMember
	Collocate     CollMember
	Deprel        string
	LogDice       float64
	TScore        float64
	MutualDist    float64
	LMI           float64
	LogLikelihood float64
	RRFScore      float64
	TextType      string
}

func (col Collocation) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Lemma         CollMember   `json:"lemma"`
		IsHead        bool         `json:"isHead"`
		Collocate     CollMember   `json:"collocate"`
		Deprel        string       `json:"deprel"`
		LogDice       roundedFloat `json:"logDice"`
		TScore        roundedFloat `json:"tScore"`
		MutualDist    roundedFloat `json:"mutualDist"`
		LMI           roundedFloat `json:"lmi"`
		LogLikelihood roundedFloat `json:"logLikelihood"`
		RRFScore      roundedFloat `json:"rrfScore"`
		TextType      string       `json:"textType"`
	}{
		Lemma:         col.Lemma,
		IsHead:        col.MutualDist > 0,
		Deprel:        col.Deprel,
		Collocate:     col.Collocate,
		LogDice:       roundedFloat(col.LogDice),
		TScore:        roundedFloat(col.TScore),
		MutualDist:    roundedFloat(col.MutualDist),
		LMI:           roundedFloat(col.LMI),
		RRFScore:      roundedFloat(col.RRFScore),
		LogLikelihood: roundedFloat(col.LogLikelihood),
		TextType:      col.TextType,
	})
}

func (ldr Collocation) Hash() string {
	hash := sha1.New()
	data := fmt.Sprintf("%s|%s|%t|%s|%s",
		ldr.Lemma.Value,
		ldr.Lemma.PoS,
		ldr.MutualDist > 0,
		ldr.Collocate.Value,
		ldr.TextType,
	)
	hash.Write([]byte(data))
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func (ldr Collocation) lemmaPropsAsString() string {
	if ldr.Lemma.PoS == "" {
		return "(-)"
	}
	return "(" + ldr.Lemma.PoS + ")"
}

func (ldr Collocation) collocatePropsAsString() string {
	if ldr.Collocate.PoS == "" {
		return "(-)"
	}
	return "(" + ldr.Collocate.PoS + ")"
}

func (ldr Collocation) textTypeAsString() string {
	if ldr.TextType != "" {
		return ldr.TextType
	}
	return "-"
}

func (ldr Collocation) formatNum(v float64) string {
	if math.IsInf(v, 1) || math.IsInf(v, -1) {
		return "-"
	}
	return fmt.Sprintf("% 3.2f", v)
}

func (ldr Collocation) formatNum4(v float64) string {
	if math.IsInf(v, 1) || math.IsInf(v, -1) {
		return "-"
	}
	return fmt.Sprintf("% 1.4f", v)
}

func (ldr Collocation) AsRow() []any {
	var arr string
	if ldr.MutualDist < 0 {
		dpr := ""
		if ldr.Deprel != "" {
			dpr = ldr.Deprel + " "
		}
		arr = fmt.Sprintf("%s\u2192", dpr)

	} else {
		dpr := ""
		if ldr.Deprel != "" {
			dpr = " " + ldr.Deprel
		}
		arr = fmt.Sprintf("\u2190%s", dpr)
	}
	return []any{
		ldr.textTypeAsString(),
		fmt.Sprintf("%s %s", ldr.Lemma.Value, ldr.lemmaPropsAsString()),
		arr,
		fmt.Sprintf("%s %s", ldr.Collocate.Value, ldr.collocatePropsAsString()),
		ldr.formatNum(ldr.TScore),
		ldr.formatNum(ldr.LogDice),
		ldr.formatNum(ldr.LMI),
		ldr.formatNum(ldr.LogLikelihood),
		ldr.formatNum4(ldr.RRFScore),
		ldr.formatNum(ldr.MutualDist),
	}
}
