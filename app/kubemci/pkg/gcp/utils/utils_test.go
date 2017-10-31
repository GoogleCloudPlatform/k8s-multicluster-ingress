// Copyright 2017 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package utils

import (
	"testing"
)

func TestGetZoneAndNameFromIGUrl(t *testing.T) {
	testCases := []struct {
		igUrl string
		name  string
		zone  string
		err   bool
	}{
		{
			// Parses as expected.
			igUrl: "https://www.googleapis.com/compute/v1/projects/fake-project/zones/zone1/instanceGroups/ig1",
			name:  "ig1",
			zone:  "zone1",
			err:   false,
		},
		{
			// Can parse partial urls
			igUrl: "projects/fake-project/zones/zone1/instanceGroups/ig1",
			name:  "ig1",
			zone:  "zone1",
			err:   false,
		},
		{
			// Generates an error on invalid urls.
			igUrl: "zone1/ig1",
			err:   true,
		},
	}
	for i, c := range testCases {
		zone, name, err := GetZoneAndNameFromIGUrl(c.igUrl)
		if (err != nil) != c.err {
			t.Fatalf("test %d: expected err: %v, got: %s", i, c.err, err)
		}
		if c.err {
			continue
		}
		if name != c.name {
			t.Errorf("test %d: expected name: %s, got: %s", i, c.name, name)
		}
		if zone != c.zone {
			t.Errorf("test %d: expected zone: %s, got: %s", i, c.zone, zone)
		}
	}
}

func TestGetZoneAndNameFromInstanceUrl(t *testing.T) {
	testCases := []struct {
		url  string
		name string
		zone string
		err  bool
	}{
		{
			// Parses as expected.
			url:  "https://www.googleapis.com/compute/v1/projects/fake-project/zones/zone1/instances/instance1",
			name: "instance1",
			zone: "zone1",
			err:  false,
		},
		{
			// Can parse partial urls
			url:  "projects/fake-project/zones/zone1/instances/instance1",
			name: "instance1",
			zone: "zone1",
			err:  false,
		},
		{
			// Generates an error on invalid urls.
			url: "zone1/ig1",
			err: true,
		},
	}
	for i, c := range testCases {
		zone, name, err := GetZoneAndNameFromInstanceUrl(c.url)
		if (err != nil) != c.err {
			t.Fatalf("test %d: expected err: %v, got: %s", i, c.err, err)
		}
		if c.err {
			continue
		}
		if name != c.name {
			t.Errorf("test %d: expected name: %s, got: %s", i, c.name, name)
		}
		if zone != c.zone {
			t.Errorf("test %d: expected zone: %s, got: %s", i, c.zone, zone)
		}
	}
}

func TestGetNameFromUrl(t *testing.T) {
	testCases := []struct {
		url  string
		name string
		err  bool
	}{
		{
			// Parses as expected.
			url:  "https://www.googleapis.com/compute/v1/projects/fake-project/zones/zone1/zone/zone1",
			name: "zone1",
			err:  false,
		},
		{
			// Can parse partial urls
			url:  "projects/fake-project/zones/zone1/instances/instance1",
			name: "instance1",
			err:  false,
		},
		{
			// Generates an error on invalid urls.
			url: "ig1",
			err: true,
		},
	}
	for i, c := range testCases {
		name, err := GetNameFromUrl(c.url)
		if (err != nil) != c.err {
			t.Fatalf("test %d: expected err: %v, got: %s", i, c.err, err)
		}
		if c.err {
			continue
		}
		if name != c.name {
			t.Errorf("test %d: expected name: %s, got: %s", i, c.name, name)
		}
	}
}
