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
	"fmt"
	"strings"
)

// Returns the zone and name of the instance group from a GCP instance group URL.
func GetZoneAndNameFromIGUrl(igUrl string) (string, string, error) {
	// Split the string by "/instanceGroups/".
	components := strings.Split(igUrl, "/instanceGroups/")
	if len(components) != 2 {
		return "", "", fmt.Errorf("error in parsing instance groups URL: %s, expected it to contain /instanceGroups", igUrl)
	}
	zoneUrl := components[0]
	name := components[1]
	zone, err := GetNameFromUrl(zoneUrl)
	if err != nil {
		return "", "", fmt.Errorf("error in parsing zone name from its url: %s", err)
	}
	return zone, name, nil
}

// Returns the zone and name of the instance from a GCP instance URL.
func GetZoneAndNameFromInstanceUrl(instanceUrl string) (string, string, error) {
	// Split the string by "/instances/".
	components := strings.Split(instanceUrl, "/instances/")
	if len(components) != 2 {
		return "", "", fmt.Errorf("error in parsing instance URL: %s, expected it to contain /instances", instanceUrl)
	}
	zoneUrl := components[0]
	name := components[1]
	zone, err := GetNameFromUrl(zoneUrl)
	if err != nil {
		return "", "", fmt.Errorf("error in parsing zone name from its url: %s", err)
	}
	return zone, name, nil
}

// Returns the zone and name of the instance from a GCP instance URL.
func GetNameFromUrl(url string) (string, error) {
	// To get name of a resource from its Url, split the string by "/" and use the last element.
	components := strings.Split(url, "/")
	if len(components) < 2 {
		return "", fmt.Errorf("error in parsing URL: %s, expected it to contain /", url)
	}
	return components[len(components)-1], nil

}
