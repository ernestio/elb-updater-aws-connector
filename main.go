/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package main

import (
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elb"
	ecc "github.com/ernestio/ernest-config-client"
	"github.com/nats-io/nats"
)

var nc *nats.Conn
var natsErr error

func eventHandler(m *nats.Msg) {
	var e Event

	err := e.Process(m.Data)
	if err != nil {
		return
	}

	if err = e.Validate(); err != nil {
		e.Error(err)
		return
	}

	err = updateELB(&e)
	if err != nil {
		e.Error(err)
		return
	}

	e.Complete()
}

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

func instancesToRegister(ev *Event, currentInstances []*elb.Instance) []*elb.Instance {
	var i []*elb.Instance

	for _, instance := range ev.InstanceAWSIDs {
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

func instancesToDeregister(ev *Event, currentInstances []*elb.Instance) []*elb.Instance {
	var i []*elb.Instance

	for _, ci := range currentInstances {
		exists := false
		for _, instance := range ev.InstanceAWSIDs {
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

func updateELB(ev *Event) error {
	creds := credentials.NewStaticCredentials(ev.DatacenterAccessKey, ev.DatacenterAccessToken, "")
	svc := elb.New(session.New(), &aws.Config{
		Region:      aws.String(ev.DatacenterRegion),
		Credentials: creds,
	})

	req := elb.DescribeLoadBalancersInput{
		LoadBalancerNames: []*string{aws.String(ev.ELBName)},
	}

	resp, err := svc.DescribeLoadBalancers(&req)
	if err != nil {
		return err
	}

	if len(resp.LoadBalancerDescriptions) != 1 {
		return errors.New("Could not find ELB")
	}

	// Instances to add and remove
	rreq := elb.RegisterInstancesWithLoadBalancerInput{
		LoadBalancerName: aws.String(ev.ELBName),
		Instances:        instancesToRegister(ev, resp.LoadBalancerDescriptions[0].Instances),
	}

	_, err = svc.RegisterInstancesWithLoadBalancer(&rreq)
	if err != nil {
		return err
	}

	drreq := elb.DeregisterInstancesFromLoadBalancerInput{
		LoadBalancerName: aws.String(ev.ELBName),
		Instances:        instancesToDeregister(ev, resp.LoadBalancerDescriptions[0].Instances),
	}

	_, err = svc.DeregisterInstancesFromLoadBalancer(&drreq)
	if err != nil {
		return err
	}

	// Update ports, certs and security groups

	return nil
}

func main() {
	nc = ecc.NewConfig(os.Getenv("NATS_URI")).Nats()

	fmt.Println("listening for elb.update.aws")
	nc.Subscribe("elb.update.aws", eventHandler)

	runtime.Goexit()
}
