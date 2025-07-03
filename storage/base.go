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
	"github.com/dgraph-io/badger/v4"
)

// DB is a wrapper around badger.DB providing concrete
// methods for adding/retrieving collocation information.
type DB struct {
	bdb *badger.DB
}

// Close closes the internal Badger database.
// It is necessary to perform the close especially
// in cases of data writing.
// It is possible to call the method on nil instance
// or on an uninitialized DB object, in which case
// it is a NOP.
func (db *DB) Close() error {
	if db != nil && db.bdb != nil {
		return db.bdb.Close()
	}
	return nil
}

func (db *DB) Flush() error {
	return db.bdb.DropAll()
}

func (db *DB) Size() (int64, int64) {
	return db.bdb.Size()
}
