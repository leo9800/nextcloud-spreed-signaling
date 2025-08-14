/**
 * Standalone signaling server for the Nextcloud Spreed app.
 * Copyright (C) 2020 struktur AG
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
package main

import (
	"fmt"
	"log"
	"os"
	"slices"
	"sync/atomic"

	"github.com/dlintw/goconf"
	"github.com/golang-jwt/jwt/v5"

	signaling "github.com/strukturag/nextcloud-spreed-signaling"
)

type tokensStatic struct {
	tokenKeys atomic.Value
}

func NewProxyTokensStatic(config *goconf.ConfigFile) (ProxyTokens, error) {
	result := &tokensStatic{}
	if err := result.load(config, false); err != nil {
		return nil, err
	}

	return result, nil
}

func (t *tokensStatic) setTokenKeys(keys map[string]*ProxyToken) {
	t.tokenKeys.Store(keys)
}

func (t *tokensStatic) getTokenKeys() map[string]*ProxyToken {
	return t.tokenKeys.Load().(map[string]*ProxyToken)
}

func (t *tokensStatic) Get(id string) (*ProxyToken, error) {
	tokenKeys := t.getTokenKeys()
	token := tokenKeys[id]
	return token, nil
}

func (t *tokensStatic) load(config *goconf.ConfigFile, ignoreErrors bool) error {
	options, err := signaling.GetStringOptions(config, "tokens", ignoreErrors)
	if err != nil {
		return err
	}

	tokenKeys := make(map[string]*ProxyToken)
	for id, filename := range options {
		if filename == "" {
			if !ignoreErrors {
				return fmt.Errorf("no filename given for token %s", id)
			}

			log.Printf("No filename given for token %s, ignoring", id)
			continue
		}

		keyData, err := os.ReadFile(filename)
		if err != nil {
			if !ignoreErrors {
				return fmt.Errorf("could not read public key from %s: %s", filename, err)
			}

			log.Printf("Could not read public key from %s, ignoring: %s", filename, err)
			continue
		}
		key, err := jwt.ParseRSAPublicKeyFromPEM(keyData)
		if err != nil {
			if !ignoreErrors {
				return fmt.Errorf("could not parse public key from %s: %s", filename, err)
			}

			log.Printf("Could not parse public key from %s, ignoring: %s", filename, err)
			continue
		}

		tokenKeys[id] = &ProxyToken{
			id:  id,
			key: key,
		}
	}

	if len(tokenKeys) == 0 {
		log.Printf("No token keys loaded")
	} else {
		var keyIds []string
		for k := range tokenKeys {
			keyIds = append(keyIds, k)
		}
		slices.Sort(keyIds)
		log.Printf("Enabled token keys: %v", keyIds)
	}
	t.setTokenKeys(tokenKeys)
	return nil
}

func (t *tokensStatic) Reload(config *goconf.ConfigFile) {
	if err := t.load(config, true); err != nil {
		log.Printf("Error reloading static tokens: %s", err)
	}
}

func (t *tokensStatic) Close() {
	t.setTokenKeys(map[string]*ProxyToken{})
}
