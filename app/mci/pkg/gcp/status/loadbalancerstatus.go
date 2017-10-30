package status

import (
	"encoding/json"
	"fmt"
)

// Struct to describe a multi cluster load balancer.
type LoadBalancerStatus struct {
	// Description about the resource this status description is on.
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
