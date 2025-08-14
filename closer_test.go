/**
 * Standalone signaling server for the Nextcloud Spreed app.
 * Copyright (C) 2023 struktur AG
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
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCloserMulti(t *testing.T) {
	closer := NewCloser()

	var wg sync.WaitGroup
	count := 10
	for range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-closer.C
		}()
	}

	assert.False(t, closer.IsClosed())
	closer.Close()
	assert.True(t, closer.IsClosed())
	wg.Wait()
}

func TestCloserCloseBeforeWait(t *testing.T) {
	closer := NewCloser()
	closer.Close()
	assert.True(t, closer.IsClosed())
	<-closer.C
	assert.True(t, closer.IsClosed())
}
