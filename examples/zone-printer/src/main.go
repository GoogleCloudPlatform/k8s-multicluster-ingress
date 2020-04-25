// Copyright 2018 Google Inc.
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
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"cloud.google.com/go/compute/metadata"
)

var (
	healthCheckPath = flag.String("health_check_path", "/", "path to serve health checks on")
	healthCheckPort = flag.Int("health_check_port", 8081, "port to serve health checks on")
	version         = flag.String("version", "1.0", "denotes the version of this app")

	// GCE zone that this server is deployed in.
	computeZone string

	// supports toggling of health check return code.
	killed bool
	lock   sync.RWMutex

	// port that core traffic is served on.
	mainPort = 8080
)

func main() {
	flag.Parse()

	if !metadata.OnGCE() {
		log.Println("warn: not running on gce")
	} else {
		zone, err := metadata.Zone()
		if err != nil {
			log.Fatalf("failed to get compute zone: %+v", err)
		}
		computeZone = zone
		log.Printf("info: determined zone: %q", zone)
	}

	log.Printf("starting to listen on port %d", *healthCheckPort)
	hcServer := http.NewServeMux()
	hcServer.HandleFunc(*healthCheckPath, handleHealthCheck)
	go func() {
		portStr := fmt.Sprintf(":%d", *healthCheckPort)
		log.Fatal(http.ListenAndServe(portStr, hcServer))
	}()

	log.Printf("starting to listen on port %d", mainPort)
	mainServer := http.NewServeMux()
	mainServer.HandleFunc("/", handleLocation)
	mainServer.HandleFunc("/ping", handlePing)
	mainServer.HandleFunc("/location", handleLocation)
	mainServer.HandleFunc("/toggleKill", handleKill)
	portStr := fmt.Sprintf(":%d", mainPort)
	log.Fatal(http.ListenAndServe(portStr, mainServer))
}

type pingResponse struct {
	Hostname string
	Version  string
	GCPZone  string
	Backend  string
}

func handlePing(w http.ResponseWriter, r *http.Request) {
	podHostname, err := os.Hostname()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pr := &pingResponse{Hostname: r.Host, Version: *version, GCPZone: computeZone, Backend: podHostname}
	j, err := json.Marshal(pr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(j)
}

func handleKill(w http.ResponseWriter, r *http.Request) {
	lock.Lock()
	defer lock.Unlock()

	killed = !killed
	fmt.Fprintf(w, "Successfully toggled kill status")
}

func handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	lock.RLock()
	defer lock.RUnlock()

	if !killed {
		fmt.Fprintf(w, "OK")
	} else {
		http.Error(w, "Not OK", http.StatusInternalServerError)
	}
}

func handleLocation(w http.ResponseWriter, r *http.Request) {
	var srcIP string
	if ipHeader := r.Header.Get("X-Forwarded-For"); ipHeader != "" {
		srcIP = ipHeader
	} else {
		srcIP = r.RemoteAddr
	}
	log.Printf("received request method=%s path=%q src=%q", r.Method, r.URL.Path, srcIP)

	if computeZone == "" {
		fmt.Fprintf(w, `<!DOCTYPE html>
				<h1>Cannot determine the compute zone :(</h1>
				<p>Is it running on a Google Compute Engine instance?</p>`)
		return
	}

	region := computeZone[:strings.LastIndex(computeZone, "-")]
	dc, ok := datacenters[region]
	if !ok {
		fmt.Fprintf(w, `<!DOCTYPE html>
		<h4>Welcome from Google Cloud datacenters at:<h4>
		<h1>%s!</h1>`, computeZone)
		return
	}
	fmt.Fprintf(w, `<!DOCTYPE html>
		<h4>Welcome from Google Cloud datacenters at:</h4>
		<h1>%s</h1>
		<h3>You are now connected to &quot;%s&quot;</h3>
		<img src="%s" style="width: 640px; height: auto; border: 1px solid black"/>`, dc.location, computeZone, dc.flagURL)
}

var (
	// datacenters adopted from https://cloud.google.com/compute/docs/regions-zones/
	datacenters = map[string]struct {
		location string
		flagURL  string // flag images must be public domain
	}{
		"northamerica-northeast1": {
			location: "Montréal, Canada",
			flagURL:  "https://upload.wikimedia.org/wikipedia/commons/d/d9/Flag_of_Canada_%28Pantone%29.svg",
		},
		"us-central1": {
			location: "Council Bluffs, Iowa, USA",
			flagURL:  "https://upload.wikimedia.org/wikipedia/en/a/a4/Flag_of_the_United_States.svg",
		},
		"us-west1": {
			location: "The Dalles, Oregon, USA",
			flagURL:  "https://upload.wikimedia.org/wikipedia/en/a/a4/Flag_of_the_United_States.svg",
		},
		"us-east4": {
			location: "Ashburn, Virginia, USA",
			flagURL:  "https://upload.wikimedia.org/wikipedia/en/a/a4/Flag_of_the_United_States.svg",
		},
		"us-east1": {
			location: "Moncks Corner, South Carolina, USA",
			flagURL:  "https://upload.wikimedia.org/wikipedia/en/a/a4/Flag_of_the_United_States.svg",
		},
		"southamerica-east1": {
			location: "São Paulo, Brazil",
			flagURL:  "https://upload.wikimedia.org/wikipedia/en/0/05/Flag_of_Brazil.svg",
		},
		"europe-west1": {
			location: "St. Ghislain, Belgium",
			flagURL:  "https://upload.wikimedia.org/wikipedia/commons/6/65/Flag_of_Belgium.svg",
		},
		"europe-west2": {
			location: "London, U.K.",
			flagURL:  "https://upload.wikimedia.org/wikipedia/en/a/ae/Flag_of_the_United_Kingdom.svg",
		},
		"europe-west3": {
			location: "Frankfurt, Germany",
			flagURL:  "https://upload.wikimedia.org/wikipedia/en/b/ba/Flag_of_Germany.svg",
		},
		"europe-west4": {
			location: "Eemshaven, Netherlands",
			flagURL:  "https://upload.wikimedia.org/wikipedia/commons/2/20/Flag_of_the_Netherlands.svg",
		},
		"asia-south1": {
			location: "Mumbai, India",
			flagURL:  "https://upload.wikimedia.org/wikipedia/en/4/41/Flag_of_India.svg",
		},
		"asia-southeast1": {
			location: "Jurong West, Singapore",
			flagURL:  "https://upload.wikimedia.org/wikipedia/commons/4/48/Flag_of_Singapore.svg",
		},
		"asia-east1": {
			location: "Changhua County, Taiwan",
			flagURL:  "https://upload.wikimedia.org/wikipedia/commons/7/72/Flag_of_the_Republic_of_China.svg",
		},
		"asia-northeast1": {
			location: "Tokyo, Japan",
			flagURL:  "https://upload.wikimedia.org/wikipedia/en/9/9e/Flag_of_Japan.svg",
		},
		"australia-southeast1": {
			location: "Sydney, Australia",
			flagURL:  "https://upload.wikimedia.org/wikipedia/en/b/b9/Flag_of_Australia.svg",
		},
	}
)
