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
	"fmt"
)

// ----

type GroupingKey string

func (gk GroupingKey) String() string {
	return fmt.Sprintf("GroupingKey(%s)", string(gk))
}

// ----

type TokenFreq struct {
	Lemma    string
	PoS      UDPoS
	Freq     int
	TextType TextType
}

func (otf TokenFreq) IsZero() bool {
	return otf.Lemma == ""
}

func (otf TokenFreq) String() string {
	return fmt.Sprintf(
		"TokenFreq(lemma: %s, pos: %s, freq: %d, tt: %s (%x))",
		otf.Lemma,
		UDPoSMapping.GetRev(otf.PoS.Byte()),
		otf.Freq,
		otf.TextType.Readable,
		otf.TextType.Raw,
	)
}

func (otf TokenFreq) HasPoS() bool {
	return otf.PoS.IsValid()
}

// Key produces a key dependent on all value's properties except for the frequency. It allows
// e.g. for incremental calculation of lemma's frequency as we process a text source file.
func (otf TokenFreq) Key() GroupingKey {
	if otf.PoS.IsValid() {
		return GroupingKey(fmt.Sprintf("%x|%s|%x", otf.TextType.Byte(), otf.Lemma, otf.PoS.Byte()))
	}
	return GroupingKey(fmt.Sprintf("%x|%s|-", otf.TextType.Byte(), otf.Lemma))
}

// LemmaKey generates a key dependent just on the actual lemma (i.e. no PoS etc.).
// This allows for grouping different (e.g. different text types or deprel) TokenFreq
// instances together if needed.
func (otf TokenFreq) LemmaKey() string {
	return otf.Lemma
}

func (otf *TokenFreq) UpdateFreq(freq int) {
	otf.Freq += freq
}

func SumTokenFreqs(items []TokenFreq) int {
	ans := 0
	for _, item := range items {
		ans += item.Freq
	}
	return ans
}

// -------

type CollocFreq struct {
	Lemma1   string
	PoS1     UDPoS
	Deprel   UDDeprel
	Lemma2   string
	PoS2     UDPoS
	Freq     int
	AVGDist  float64
	TextType TextType
}

func (cf CollocFreq) String() string {
	return fmt.Sprintf(
		"CollocFreq(lemma1: %s, pos1: %s, deprel: %s, lemma2: %s, pos2: %s, freq: %d, tt: %s (%x))",
		cf.Lemma1,
		UDPoSMapping.GetRev(cf.PoS1.Byte()),
		UDDeprelMapping.GetRev(cf.Deprel.AsUint16()),
		cf.Lemma2,
		UDPoSMapping.GetRev(cf.PoS2.Byte()),
		cf.Freq,
		cf.TextType.Readable,
		cf.TextType.Raw,
	)
}

func (cf *CollocFreq) UpdateFreqAndDist(freq, dist int) {
	// create a continuous average of distance between lemma1 and lemma2
	cf.AVGDist = (float64(cf.Freq)*cf.AVGDist + float64(dist)) / float64(cf.Freq+1)
	cf.Freq += freq
}

func (cf CollocFreq) Key() GroupingKey {
	headDep := "h"
	if cf.AVGDist < 0 {
		headDep = "d"
	}
	if cf.PoS1.IsValid() && cf.PoS2.IsValid() {
		return GroupingKey(
			fmt.Sprintf(
				"%x|%s|%s|%x|%x|%s|%x",
				cf.TextType.Byte(), cf.Lemma1, headDep, cf.PoS1.Byte(), cf.Deprel.AsUint16(), cf.Lemma2, cf.PoS2.Byte()))
	}
	return GroupingKey(fmt.Sprintf("%x|%s|%s|%x|%s", cf.TextType.Byte(), cf.Lemma1, headDep, cf.Deprel.AsUint16(), cf.Lemma2))
}

func (cf CollocFreq) Lemma1Key() string {
	return cf.Lemma1
}

func (cf CollocFreq) Lemma2Key() string {
	return cf.Lemma2
}

func SumCollocFreqs(items []CollocFreq) int {
	ans := 0
	for _, item := range items {
		ans += item.Freq
	}
	return ans
}
