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

func updateELBInstances(svc *elb.ELB, lb *elb.LoadBalancerDescription, ni []string) error {
	// Instances to remove
	drreq := elb.DeregisterInstancesFromLoadBalancerInput{
		LoadBalancerName: lb.LoadBalancerName,
		Instances:        instancesToDeregister(ni, lb.Instances),
	}

	_, err := svc.DeregisterInstancesFromLoadBalancer(&drreq)
	if err != nil {
		return err
	}

	// Instances to add
	rreq := elb.RegisterInstancesWithLoadBalancerInput{
		LoadBalancerName: lb.LoadBalancerName,
		Instances:        instancesToRegister(ni, lb.Instances),
	}

	_, err = svc.RegisterInstancesWithLoadBalancer(&rreq)

	return err
}

func updateELBListeners(svc *elb.ELB, lb *elb.LoadBalancerDescription, nl []Port) error {
	dlreq := elb.DeleteLoadBalancerListenersInput{
		LoadBalancerName:  lb.LoadBalancerName,
		LoadBalancerPorts: listenersToDelete(nl, lb.ListenerDescriptions),
	}

	_, err := svc.DeleteLoadBalancerListeners(&dlreq)
	if err != nil {
		return err
	}

	clreq := elb.CreateLoadBalancerListenersInput{
		LoadBalancerName: lb.LoadBalancerName,
		Listeners:        listenersToCreate(nl, lb.ListenerDescriptions),
	}

	_, err = svc.CreateLoadBalancerListeners(&clreq)

	return err
}

func updateELBNetworks(svc *elb.ELB, lb *elb.LoadBalancerDescription, nl []string) error {
	dsreq := elb.DetachLoadBalancerFromSubnetsInput{
		LoadBalancerName: lb.LoadBalancerName,
		Subnets:          subnetsToDetach(nl, lb.Subnets),
	}

	_, err := svc.DetachLoadBalancerFromSubnets(&dsreq)
	if err != nil {
		return err
	}

	csreq := elb.AttachLoadBalancerToSubnetsInput{
		LoadBalancerName: lb.LoadBalancerName,
		Subnets:          subnetsToAttach(nl, lb.Subnets),
	}

	_, err = svc.AttachLoadBalancerToSubnets(&csreq)

	return err
}

func updateELBSecurityGroups(svc *elb.ELB, lb *elb.LoadBalancerDescription, nsg []string) error {
	var sgs []*string
	for _, sg := range nsg {
		sgs = append(sgs, aws.String(sg))
	}

	req := elb.ApplySecurityGroupsToLoadBalancerInput{
		LoadBalancerName: lb.LoadBalancerName,
		SecurityGroups:   sgs,
	}

	_, err := svc.ApplySecurityGroupsToLoadBalancer(&req)

	return err
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

	lb := resp.LoadBalancerDescriptions[0]

	// Update ports, certs and security groups & networks
	err = updateELBInstances(svc, lb, ev.InstanceAWSIDs)
	if err != nil {
		return err
	}

	err = updateELBListeners(svc, lb, ev.ELBPorts)
	if err != nil {
		return err
	}

	err = updateELBNetworks(svc, lb, ev.NetworkAWSIDs)
	if err != nil {
		return err
	}

	err = updateELBSecurityGroups(svc, lb, ev.SecurityGroupAWSIDs)
	if err != nil {
		return err
	}

	return nil
}

func main() {
	nc = ecc.NewConfig(os.Getenv("NATS_URI")).Nats()

	fmt.Println("listening for elb.update.aws")
	nc.Subscribe("elb.update.aws", eventHandler)

	runtime.Goexit()
}
