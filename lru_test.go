/**
 * Standalone signaling server for the Nextcloud Spreed app.
 * Copyright (C) 2017 struktur AG
 *
 * @author Joachim Bauch <bauch@struktur.de>
 *
 * @license GNU AGPL version 3 or any later version
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */
package signaling

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLruUnbound(t *testing.T) {
	assert := assert.New(t)
	lru := NewLruCache(0)
	count := 10
	for i := range count {
		key := fmt.Sprintf("%d", i)
		lru.Set(key, i)
	}
	assert.Equal(count, lru.Len())
	for i := range count {
		key := fmt.Sprintf("%d", i)
		if value := lru.Get(key); assert.NotNil(value, "No value found for %s", key) {
			assert.EqualValues(i, value)
		}
	}
	// The first key ("0") is now the oldest.
	lru.RemoveOldest()
	assert.Equal(count-1, lru.Len())
	for i := range count {
		key := fmt.Sprintf("%d", i)
		value := lru.Get(key)
		if i == 0 {
			assert.Nil(value, "The value for key %s should have been removed", key)
			continue
		} else if assert.NotNil(value, "No value found for %s", key) {
			assert.EqualValues(i, value)
		}
	}

	// NOTE: Key "0" no longer exists below, so make sure to not set it again.

	// Using the same keys will update the ordering.
	for i := count - 1; i >= 1; i-- {
		key := fmt.Sprintf("%d", i)
		lru.Set(key, i)
	}
	assert.Equal(count-1, lru.Len())
	// NOTE: The same ordering as the Set calls above.
	for i := count - 1; i >= 1; i-- {
		key := fmt.Sprintf("%d", i)
		if value := lru.Get(key); assert.NotNil(value, "No value found for %s", key) {
			assert.EqualValues(i, value)
		}
	}

	// The last key ("9") is now the oldest.
	lru.RemoveOldest()
	assert.Equal(count-2, lru.Len())
	for i := range count {
		key := fmt.Sprintf("%d", i)
		value := lru.Get(key)
		if i == 0 || i == count-1 {
			assert.Nil(value, "The value for key %s should have been removed", key)
			continue
		} else if assert.NotNil(value, "No value found for %s", key) {
			assert.EqualValues(i, value)
		}
	}

	// Remove an arbitrary key from the cache
	key := fmt.Sprintf("%d", count/2)
	lru.Remove(key)
	assert.Equal(count-3, lru.Len())
	for i := range count {
		key := fmt.Sprintf("%d", i)
		value := lru.Get(key)
		if i == 0 || i == count-1 || i == count/2 {
			assert.Nil(value, "The value for key %s should have been removed", key)
			continue
		} else if assert.NotNil(value, "No value found for %s", key) {
			assert.EqualValues(i, value)
		}
	}
}

func TestLruBound(t *testing.T) {
	assert := assert.New(t)
	size := 2
	lru := NewLruCache(size)
	count := 10
	for i := range count {
		key := fmt.Sprintf("%d", i)
		lru.Set(key, i)
	}
	assert.Equal(size, lru.Len())
	// Only the last "size" entries have been stored.
	for i := range count {
		key := fmt.Sprintf("%d", i)
		value := lru.Get(key)
		if i < count-size {
			assert.Nil(value, "The value for key %s should have been removed", key)
			continue
		} else if assert.NotNil(value, "No value found for %s", key) {
			assert.EqualValues(i, value)
		}
	}
}
