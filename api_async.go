/**
 * Standalone signaling server for the Nextcloud Spreed app.
 * Copyright (C) 2022 struktur AG
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
	"encoding/json"
	"fmt"
	"time"
)

type AsyncMessage struct {
	SendTime time.Time `json:"sendtime"`

	Type string `json:"type"`

	Message *ServerMessage `json:"message,omitempty"`

	Room *BackendServerRoomRequest `json:"room,omitempty"`

	Permissions []Permission `json:"permissions,omitempty"`

	AsyncRoom *AsyncRoomMessage `json:"asyncroom,omitempty"`

	SendOffer *SendOfferMessage `json:"sendoffer,omitempty"`

	Id string `json:"id"`
}

func (m *AsyncMessage) String() string {
	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Sprintf("Could not serialize %#v: %s", m, err)
	}
	return string(data)
}

type AsyncRoomMessage struct {
	Type string `json:"type"`

	SessionId  PublicSessionId `json:"sessionid,omitempty"`
	ClientType ClientType      `json:"clienttype,omitempty"`
}

type SendOfferMessage struct {
	MessageId string                    `json:"messageid,omitempty"`
	SessionId PublicSessionId           `json:"sessionid"`
	Data      *MessageClientMessageData `json:"data"`
}
