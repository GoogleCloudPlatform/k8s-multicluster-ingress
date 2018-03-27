package status

import (
	"encoding/json"
	"fmt"

	"github.com/golang/glog"
)

// Struct to describe a multi cluster load balancer.
type LoadBalancerStatus struct {
	// Human readable description for this status.
	// If we are using the description field of a GCP resource to store this status,
	// then this description field can be used to store the description about that GCP resource.
	Description string
	// Name of the load balancer which is described by this.
	LoadBalancerName string
	// Name of the clusters across which this load balancer is spread.
	Clusters []string
	// IP Address of this load balancer.
	IPAddress string
	// TODO: Store errors that were generated during creating and deleting this load balancer.
}

func (s LoadBalancerStatus) ToString() (string, error) {
	jsonValue, err := json.Marshal(s)
	if err != nil {
		return "", fmt.Errorf("error %s in marshalling status description %v", err, s)
	}
	return string(jsonValue), nil
}

func FromString(str string) (*LoadBalancerStatus, error) {
	var s LoadBalancerStatus
	if err := json.Unmarshal([]byte(str), &s); err != nil {
		return nil, fmt.Errorf("error %s in unmarshalling string %s", err, str)
	}
	return &s, nil
}

// RemoveClusters removes the given list of clusters from the given status string.
// Returns NotFoundError if the given statusStr is not a valid LoadBalancerStatus.
func RemoveClusters(statusStr string, clustersToRemove []string) (string, error) {
	status, err := FromString(statusStr)
	if err != nil {
		glog.V(2).Infof("error in parsing description %s: %s. Ignoring the error by assuming that this is because status is on some other resource and continuing", statusStr, err)
		return statusStr, nil
	}
	removeMap := map[string]bool{}
	for _, v := range clustersToRemove {
		removeMap[v] = true
	}
	var newClusters []string
	for _, v := range status.Clusters {
		if !removeMap[v] {
			newClusters = append(newClusters, v)
		}
	}
	status.Clusters = newClusters
	newDesc, err := status.ToString()
	if err != nil {
		return statusStr, fmt.Errorf("unexpected error in converting status to string. status: %s, err: %s", status, err)
	}
	return newDesc, nil
}
