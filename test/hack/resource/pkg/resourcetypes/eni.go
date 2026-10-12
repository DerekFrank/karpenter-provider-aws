/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package resourcetypes

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/samber/lo"
	"go.uber.org/multierr"
)

const (
	branchENIDescription  = "aws-k8s-branch-eni"
	eniOwnerTag           = "eks:eni:owner"
	vpcResourceController = "eks-vpc-resource-controller"
	branchENIInstanceTag  = "node.k8s.amazonaws.com/instance_id"
	k8sClusterTagPrefix   = "kubernetes.io/cluster/"
)

type ENI struct {
	ec2Client *ec2.Client
}

func NewENI(ec2Client *ec2.Client) *ENI {
	return &ENI{ec2Client: ec2Client}
}

func (e *ENI) String() string {
	return "ElasticNetworkInterface"
}

func (e *ENI) Global() bool {
	return false
}

func (e *ENI) GetExpired(ctx context.Context, expirationTime time.Time, excludedClusters []string) (ids []string, err error) {
	enis, err := e.getAllENIs(ctx, &ec2.DescribeNetworkInterfacesInput{
		Filters: []ec2types.Filter{
			{
				Name:   lo.ToPtr("tag-key"),
				Values: []string{k8sClusterTag},
			},
		},
	})
	if err != nil {
		return ids, err
	}

	for _, ni := range enis {
		clusterName, found := lo.Find(ni.TagSet, func(tag ec2types.Tag) bool {
			return *tag.Key == k8sClusterTag
		})
		if found && slices.Contains(excludedClusters, lo.FromPtr(clusterName.Value)) {
			continue
		}
		creationDate, found := lo.Find(ni.TagSet, func(tag ec2types.Tag) bool {
			return *tag.Key == "node.k8s.amazonaws.com/createdAt"
		})
		if !found {
			continue
		}
		creationTime, err := time.Parse(time.RFC3339, *creationDate.Value)
		if err != nil {
			continue
		}
		if ni.Status == ec2types.NetworkInterfaceStatusAvailable && creationTime.Before(expirationTime) {
			ids = append(ids, lo.FromPtr(ni.NetworkInterfaceId))
		}
	}

	branchIDs, err := e.getExpiredBranchENIs(ctx, excludedClusters)
	return append(ids, branchIDs...), err
}

// getExpiredBranchENIs returns unattached VPC resource controller branch ENIs from E2E clusters whose node no longer exists.
// Branch ENIs carry no creation timestamp, so the node being gone stands in for the expiration time. Leaked branch ENIs block
// deletion of the cluster's subnets and security groups, which leaves the eksctl stack in DELETE_FAILED.
func (e *ENI) getExpiredBranchENIs(ctx context.Context, excludedClusters []string) (ids []string, err error) {
	enis, err := e.getAllENIs(ctx, &ec2.DescribeNetworkInterfacesInput{
		Filters: []ec2types.Filter{
			{Name: lo.ToPtr("description"), Values: []string{branchENIDescription}},
			{Name: lo.ToPtr("status"), Values: []string{string(ec2types.NetworkInterfaceStatusAvailable)}},
			{Name: lo.ToPtr("tag:" + eniOwnerTag), Values: []string{vpcResourceController}},
		},
	})
	if err != nil {
		return ids, err
	}
	enis = lo.Filter(enis, func(ni ec2types.NetworkInterface, _ int) bool {
		clusterName, ok := branchENIClusterName(ni)
		return ok && isE2ECluster(clusterName) && !slices.Contains(excludedClusters, clusterName)
	})
	liveInstances, err := e.getLiveInstanceIDs(ctx, lo.Uniq(lo.FilterMap(enis, func(ni ec2types.NetworkInterface, _ int) (string, bool) {
		return branchENIInstanceID(ni)
	})))
	if err != nil {
		return ids, err
	}
	return orphanedBranchENIs(enis, liveInstances), nil
}

// getLiveInstanceIDs returns the subset of instanceIDs that exist and are not terminated
func (e *ENI) getLiveInstanceIDs(ctx context.Context, instanceIDs []string) (map[string]struct{}, error) {
	live := map[string]struct{}{}
	// EC2 accepts at most 200 values per filter
	for _, chunk := range lo.Chunk(instanceIDs, 200) {
		paginator := ec2.NewDescribeInstancesPaginator(e.ec2Client, &ec2.DescribeInstancesInput{
			Filters: []ec2types.Filter{
				{Name: lo.ToPtr("instance-id"), Values: chunk},
				{Name: lo.ToPtr("instance-state-name"), Values: []string{
					string(ec2types.InstanceStateNamePending),
					string(ec2types.InstanceStateNameRunning),
					string(ec2types.InstanceStateNameShuttingDown),
					string(ec2types.InstanceStateNameStopping),
					string(ec2types.InstanceStateNameStopped),
				}},
			},
		})
		for paginator.HasMorePages() {
			page, err := paginator.NextPage(ctx)
			if err != nil {
				return nil, err
			}
			for _, reservation := range page.Reservations {
				for _, instance := range reservation.Instances {
					live[lo.FromPtr(instance.InstanceId)] = struct{}{}
				}
			}
		}
	}
	return live, nil
}

func (e *ENI) CountAll(ctx context.Context) (count int, err error) {
	enis, err := e.getAllENIs(ctx, &ec2.DescribeNetworkInterfacesInput{})
	if err != nil {
		return 0, err
	}

	return len(enis), err
}

func (e *ENI) Get(ctx context.Context, clusterName string) (ids []string, err error) {
	enis, err := e.getAllENIs(ctx, &ec2.DescribeNetworkInterfacesInput{
		Filters: []ec2types.Filter{
			{
				Name:   lo.ToPtr("tag:" + k8sClusterTag),
				Values: []string{clusterName},
			},
		},
	})
	if err != nil {
		return ids, err
	}

	branchENIs, err := e.getAllENIs(ctx, &ec2.DescribeNetworkInterfacesInput{
		Filters: []ec2types.Filter{
			{Name: lo.ToPtr("tag-key"), Values: []string{k8sClusterTagPrefix + clusterName}},
			{Name: lo.ToPtr("tag:" + eniOwnerTag), Values: []string{vpcResourceController}},
		},
	})
	if err != nil {
		return ids, err
	}

	for _, ni := range append(enis, branchENIs...) {
		ids = append(ids, lo.FromPtr(ni.NetworkInterfaceId))
	}
	return lo.Uniq(ids), err
}

// Cleanup any old ENIs that were managed by Karpenter or were provisioned as part of testing
// We execute these in serial since we will most likely get rate limited if we try to delete these too aggressively
func (e *ENI) Cleanup(ctx context.Context, ids []string) ([]string, error) {
	var deleted []string
	var errs error
	for i := range ids {
		_, err := e.ec2Client.DeleteNetworkInterface(ctx, &ec2.DeleteNetworkInterfaceInput{
			NetworkInterfaceId: aws.String(ids[i]),
		})
		if err != nil {
			errs = multierr.Append(errs, err)
			continue
		}
		deleted = append(deleted, ids[i])
	}

	return deleted, errs
}

func (e *ENI) getAllENIs(ctx context.Context, params *ec2.DescribeNetworkInterfacesInput) (enis []ec2types.NetworkInterface, err error) {
	paginator := ec2.NewDescribeNetworkInterfacesPaginator(e.ec2Client, params)

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return enis, err
		}
		enis = append(enis, page.NetworkInterfaces...)
	}

	return enis, nil
}

func branchENIClusterName(ni ec2types.NetworkInterface) (string, bool) {
	tag, found := lo.Find(ni.TagSet, func(t ec2types.Tag) bool {
		return strings.HasPrefix(lo.FromPtr(t.Key), k8sClusterTagPrefix)
	})
	if !found {
		return "", false
	}
	return strings.TrimPrefix(lo.FromPtr(tag.Key), k8sClusterTagPrefix), true
}

func branchENIInstanceID(ni ec2types.NetworkInterface) (string, bool) {
	tag, found := lo.Find(ni.TagSet, func(t ec2types.Tag) bool {
		return lo.FromPtr(t.Key) == branchENIInstanceTag
	})
	return lo.FromPtr(tag.Value), found && lo.FromPtr(tag.Value) != ""
}

// orphanedBranchENIs returns the ids of the ENIs whose node is not in liveInstances.
// ENIs without a node tag are skipped, since we can't tell whether they are still in use.
func orphanedBranchENIs(enis []ec2types.NetworkInterface, liveInstances map[string]struct{}) (ids []string) {
	for _, ni := range enis {
		instanceID, ok := branchENIInstanceID(ni)
		if !ok {
			continue
		}
		if _, live := liveInstances[instanceID]; !live {
			ids = append(ids, lo.FromPtr(ni.NetworkInterfaceId))
		}
	}
	return ids
}
