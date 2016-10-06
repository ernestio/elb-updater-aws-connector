/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package main

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elb"
)

func mapListeners(ev *Event) []*elb.Listener {
	var l []*elb.Listener

	for _, port := range ev.ELBPorts {
		l = append(l, &elb.Listener{
			Protocol:         aws.String(port.Protocol),
			LoadBalancerPort: aws.Int64(port.FromPort),
			InstancePort:     aws.Int64(port.ToPort),
			InstanceProtocol: aws.String(port.Protocol),
			SSLCertificateId: aws.String(port.SSLCertID),
		})
	}

	return l
}

func portInUse(listeners []*elb.ListenerDescription, port int64) bool {
	for _, l := range listeners {
		if *l.Listener.LoadBalancerPort == port {
			return true
		}
	}

	return false
}

func portRemoved(ports []Port, listener *elb.ListenerDescription) bool {
	for _, p := range ports {
		if p.FromPort == *listener.Listener.LoadBalancerPort {
			return false
		}
	}

	return true
}

func instancesToRegister(newInstances []string, currentInstances []*elb.Instance) []*elb.Instance {
	var i []*elb.Instance

	for _, instance := range newInstances {
		exists := false
		for _, ci := range currentInstances {
			if instance == *ci.InstanceId {
				exists = true
			}
		}
		if exists != true {
			i = append(i, &elb.Instance{InstanceId: aws.String(instance)})
		}
	}

	return i
}

func instancesToDeregister(newInstances []string, currentInstances []*elb.Instance) []*elb.Instance {
	var i []*elb.Instance

	for _, ci := range currentInstances {
		exists := false
		for _, instance := range newInstances {
			if *ci.InstanceId == instance {
				exists = true
			}
		}
		if exists != true {
			i = append(i, &elb.Instance{InstanceId: ci.InstanceId})
		}
	}

	return i
}

func listenersToDelete(newListeners []Port, currentListeners []*elb.ListenerDescription) []*int64 {
	var l []*int64

	for _, cl := range currentListeners {
		if portRemoved(newListeners, cl) {
			l = append(l, cl.Listener.LoadBalancerPort)
		}
	}

	return l
}

func listenersToCreate(newListeners []Port, currentListeners []*elb.ListenerDescription) []*elb.Listener {
	var l []*elb.Listener

	for _, listener := range newListeners {

		if portInUse(currentListeners, listener.FromPort) != true {
			l = append(l, &elb.Listener{
				Protocol:         aws.String(listener.Protocol),
				LoadBalancerPort: aws.Int64(listener.FromPort),
				InstancePort:     aws.Int64(listener.ToPort),
				InstanceProtocol: aws.String(listener.Protocol),
				SSLCertificateId: aws.String(listener.SSLCertID),
			})
		}
	}

	return l
}

func subnetsToAttach(newSubnets []string, currentSubnets []*string) []*string {
	var s []*string

	for _, subnet := range newSubnets {
		exists := false
		for _, cs := range currentSubnets {
			if subnet == *cs {
				exists = true
			}
		}
		if exists != true {
			s = append(s, aws.String(subnet))
		}
	}

	return s
}

func subnetsToDetach(newSubnets []string, currentSubnets []*string) []*string {
	var s []*string

	for _, cs := range currentSubnets {
		exists := false
		for _, subnet := range newSubnets {
			if *cs == subnet {
				exists = true
			}
		}
		if exists != true {
			s = append(s, cs)
		}
	}

	return s
}
