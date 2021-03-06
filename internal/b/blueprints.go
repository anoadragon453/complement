// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package b

import (
	"fmt"
	"strings"
)

// Blueprint represents an entire deployment to make.
type Blueprint struct {
	// The name of the blueprint. Containers will use this name.
	Name string
	// The list of homeservers to create for this deployment.
	Homeservers []Homeserver
}

type Homeserver struct {
	// The name of this homeserver. Containers will use this name.
	Name string
	// The list of users to create on this homeserver.
	Users []User
	// The list of rooms to create on this homeserver
	Rooms []Room
}

type User struct {
	Localpart   string
	DisplayName string
	AvatarURL   string
	AccountData AccountData
}

type AccountData struct {
	Type  string
	Value map[string]interface{}
}

type Room struct {
	// The unique reference for this room. Used to link together rooms across homeservers.
	Ref        string
	Creator    string
	CreateRoom map[string]interface{}
	Events     []Event
}

type Event struct {
	Type     string
	Sender   string
	StateKey *string
	Content  map[string]interface{}
}

func MustValidate(bp Blueprint) Blueprint {
	bp2, err := Validate(bp)
	if err != nil {
		panic("MustValidate: " + err.Error())
	}
	return bp2
}

func Validate(bp Blueprint) (Blueprint, error) {
	if bp.Name == "" {
		return bp, fmt.Errorf("Blueprint must have a Name")
	}
	var err error
	for _, hs := range bp.Homeservers {
		for i, u := range hs.Users {
			if !strings.HasPrefix(u.Localpart, "@") {
				return bp, fmt.Errorf("HS %s user localpart '%s' must start with '@'", hs.Name, u.Localpart)
			}
			if strings.Contains(u.Localpart, ":") {
				return bp, fmt.Errorf("HS %s user localpart '%s' must not contain a domain", hs.Name, u.Localpart)
			}
			// strip the @
			hs.Users[i].Localpart = hs.Users[i].Localpart[1:]
		}
		for i := range hs.Rooms {
			hs.Rooms[i], err = normaliseRoom(hs.Name, hs.Rooms[i])
			if err != nil {
				return bp, err
			}
		}
	}
	return bp, nil
}

func normaliseRoom(hsName string, r Room) (Room, error) {
	var err error
	if r.Creator != "" {
		r.Creator, err = normaliseUser(r.Creator, hsName)
		if err != nil {
			return r, err
		}
	} else if r.Ref == "" {
		return r, fmt.Errorf("%s : room must have either a Ref or a Creator", hsName)
	}
	for i := range r.Events {
		r.Events[i].Sender, err = normaliseUser(r.Events[i].Sender, hsName)
		if err != nil {
			return r, err
		}
		if r.Events[i].StateKey != nil && r.Events[i].Type == "m.room.member" {
			skey, err := normaliseUser(*r.Events[i].StateKey, hsName)
			if err != nil {
				return r, err
			}
			r.Events[i].StateKey = &skey
		}
	}
	return r, nil
}

func normaliseUser(u string, hsName string) (string, error) {
	// if they did it as @foo:bar make sure :bar is the name of the HS
	if strings.Contains(u, ":") {
		if strings.HasSuffix(u, fmt.Sprintf(":%s", hsName)) {
			return u, nil
		}
		return "", fmt.Errorf("HS '%s' user '%s' must end with ':%s' or have no domain", hsName, u, hsName)
	}
	// add :domain
	if !strings.Contains(u, ":") {
		u += ":" + hsName
	}
	return u, nil
}

// Ptr returns a pointer to `in`, because Go doesn't allow you to inline this.
func Ptr(in string) *string {
	return &in
}
