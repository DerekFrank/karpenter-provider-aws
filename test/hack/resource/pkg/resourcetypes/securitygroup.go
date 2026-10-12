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
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/samber/lo"
	"go.uber.org/multierr"
)

// eksClusterSGPrefix is the name prefix of the security group EKS creates for a cluster, "eks-cluster-sg-<cluster>-<id>".
// It isn't part of the eksctl stack, and when it leaks it keeps the stack from deleting the VPC.
const eksClusterSGPrefix = "eks-cluster-sg-"

type SecurityGroup struct {
	ec2Client *ec2.Client
	clusters  *clusterChecker
}

func NewSecurityGroup(ec2Client *ec2.Client, eksClient *eks.Client) *SecurityGroup {
	return &SecurityGroup{ec2Client: ec2Client, clusters: newClusterChecker(eksClient)}
}

func (sg *SecurityGroup) String() string {
	return "SecurityGroup"
}

func (sg *SecurityGroup) Global() bool {
	return false
}

func (sg *SecurityGroup) GetExpired(ctx context.Context, expirationTime time.Time, excludedClusters []string) (ids []string, err error) {
	sgs, err := sg.getAllSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: []ec2types.Filter{
			{
				Name:   lo.ToPtr("group-name"),
				Values: []string{"security-group-drift"},
			},
		},
	})
	if err != nil {
		return ids, err
	}

	for _, sgroup := range sgs {
		clusterName, found := lo.Find(sgroup.Tags, func(tag ec2types.Tag) bool {
			return *tag.Key == karpenterTestingTag
		})
		if found && slices.Contains(excludedClusters, lo.FromPtr(clusterName.Value)) {
			continue
		}
		creationDate, found := lo.Find(sgroup.Tags, func(tag ec2types.Tag) bool {
			return *tag.Key == "creation-date"
		})
		if !found {
			continue
		}
		time, err := time.Parse(time.RFC3339, *creationDate.Value)
		if err != nil {
			continue
		}
		if time.Before(expirationTime) {
			ids = append(ids, lo.FromPtr(sgroup.GroupId))
		}
	}

	clusterSGs, err := sg.getEKSClusterSecurityGroups(ctx, "")
	if err != nil {
		return ids, err
	}
	var errs error
	for _, sgroup := range clusterSGs {
		clusterName, ok := eksClusterSGCluster(sgroup)
		if !ok || !isE2ECluster(clusterName) || slices.Contains(excludedClusters, clusterName) {
			continue
		}
		// These carry no creation time, so we only delete them once the cluster is gone
		gone, err := sg.clusters.Gone(ctx, clusterName)
		if err != nil {
			errs = multierr.Append(errs, err)
			continue
		}
		if gone {
			ids = append(ids, lo.FromPtr(sgroup.GroupId))
		}
	}
	return ids, errs
}

// getEKSClusterSecurityGroups returns the EKS-created cluster security groups, for one cluster if clusterName is set
func (sg *SecurityGroup) getEKSClusterSecurityGroups(ctx context.Context, clusterName string) ([]ec2types.SecurityGroup, error) {
	filters := []ec2types.Filter{{Name: lo.ToPtr("group-name"), Values: []string{eksClusterSGPrefix + "*"}}}
	if clusterName != "" {
		filters = append(filters, ec2types.Filter{Name: lo.ToPtr("tag:" + eksClusterSGNameTag), Values: []string{clusterName}})
	}
	return sg.getAllSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{Filters: filters})
}

// eksClusterSGCluster returns the cluster an EKS-created cluster security group belongs to
func eksClusterSGCluster(sgroup ec2types.SecurityGroup) (string, bool) {
	tag, found := lo.Find(sgroup.Tags, func(t ec2types.Tag) bool { return lo.FromPtr(t.Key) == eksClusterSGNameTag })
	clusterName := lo.FromPtr(tag.Value)
	if !found || !strings.HasPrefix(lo.FromPtr(sgroup.GroupName), eksClusterSGPrefix+clusterName+"-") {
		return "", false
	}
	return clusterName, true
}

func (sg *SecurityGroup) CountAll(ctx context.Context) (count int, err error) {
	sgs, err := sg.getAllSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{})
	if err != nil {
		return count, err
	}

	return len(sgs), err
}

func (sg *SecurityGroup) Get(ctx context.Context, clusterName string) (ids []string, err error) {
	sgs, err := sg.getAllSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: []ec2types.Filter{
			{
				Name:   lo.ToPtr("tag:" + karpenterSecurityGroupTag),
				Values: []string{clusterName},
			},
		},
	})
	if err != nil {
		return ids, err
	}

	for _, sgroup := range sgs {
		ids = append(ids, lo.FromPtr(sgroup.GroupId))
	}

	clusterSGs, err := sg.getEKSClusterSecurityGroups(ctx, clusterName)
	if err != nil {
		return ids, err
	}
	if len(clusterSGs) > 0 {
		gone, err := sg.clusters.Gone(ctx, clusterName)
		if err != nil {
			return ids, err
		}
		if gone {
			ids = append(ids, lo.Map(clusterSGs, func(g ec2types.SecurityGroup, _ int) string { return lo.FromPtr(g.GroupId) })...)
		}
	}
	return lo.Uniq(ids), nil
}

// Cleanup any old security groups that were provisioned as part of testing
// We execute these in serial since we will most likely get rate limited if we try to delete these too aggressively
func (sg *SecurityGroup) Cleanup(ctx context.Context, ids []string) ([]string, error) {
	var deleted []string
	var errs error
	for i := range ids {
		_, err := sg.ec2Client.DeleteSecurityGroup(ctx, &ec2.DeleteSecurityGroupInput{
			GroupId: aws.String(ids[i]),
		})
		if err != nil {
			errs = multierr.Append(errs, err)
			continue
		}
		deleted = append(deleted, ids[i])
	}

	return deleted, errs
}

func (sg *SecurityGroup) getAllSecurityGroups(ctx context.Context, params *ec2.DescribeSecurityGroupsInput) (sgs []ec2types.SecurityGroup, err error) {
	paginator := ec2.NewDescribeSecurityGroupsPaginator(sg.ec2Client, params)

	for paginator.HasMorePages() {
		out, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		sgs = append(sgs, out.SecurityGroups...)
	}

	return sgs, nil
}
