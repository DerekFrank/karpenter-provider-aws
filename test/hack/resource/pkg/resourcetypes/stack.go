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
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cloudformationtypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/samber/lo"
	"go.uber.org/multierr"
)

const (
	// iamStackPrefix is the name prefix of the stack that setup-cluster creates with `aws cloudformation deploy`.
	// When the changeset fails to execute, the stack is left in REVIEW_IN_PROGRESS without tags.
	iamStackPrefix = "iam-"
	// eksctlStackPrefix is the name prefix of the stacks eksctl creates for a cluster, "eksctl-<cluster>-*"
	eksctlStackPrefix = "eksctl-"
	// deleteStackInterval spaces out DeleteStack calls. Every DeleteStack starts resource-level operations,
	// which count against the same per-region limit that CreateStack/ExecuteChangeSet need in E2E runs.
	deleteStackInterval = 5 * time.Second
)

type Stack struct {
	cloudFormationClient *cloudformation.Client
	iamClient            *iam.Client
}

func NewStack(cloudFormationClient *cloudformation.Client, iamClient *iam.Client) *Stack {
	return &Stack{cloudFormationClient: cloudFormationClient, iamClient: iamClient}
}

func (s *Stack) String() string {
	return "CloudformationStacks"
}

func (s *Stack) Global() bool {
	return false
}

func (s *Stack) GetExpired(ctx context.Context, expirationTime time.Time, excludedClusters []string) (names []string, err error) {
	stacks, err := s.getAllStacks(ctx)
	if err != nil {
		return names, err
	}
	return expiredStacks(stacks, expirationTime, excludedClusters), nil
}

func (s *Stack) CountAll(ctx context.Context) (count int, err error) {
	stacks, err := s.getAllStacks(ctx)
	if err != nil {
		return count, err
	}

	return len(stacks), nil
}

func (s *Stack) Get(ctx context.Context, clusterName string) (names []string, err error) {
	stacks, err := s.getAllStacks(ctx)
	if err != nil {
		return names, err
	}

	return clusterStacks(stacks, clusterName), nil
}

// Cleanup any old stacks that were provisioned as part of testing
// We execute these in serial, spaced by deleteStackInterval, since every deletion starts resource-level operations
// that share a per-region limit with the stack creations of running E2E tests
func (s *Stack) Cleanup(ctx context.Context, names []string) ([]string, error) {
	stacks, err := s.getAllStacks(ctx)
	if err != nil {
		return nil, err
	}
	stacksByName := lo.KeyBy(stacks, func(stack cloudformationtypes.Stack) string { return lo.FromPtr(stack.StackName) })

	var deleted []string
	var errs error
	for i := range names {
		if i > 0 {
			select {
			case <-ctx.Done():
				return deleted, multierr.Append(errs, ctx.Err())
			case <-time.After(deleteStackInterval):
			}
		}
		if stack, ok := stacksByName[names[i]]; ok && stack.StackStatus == cloudformationtypes.StackStatusDeleteFailed {
			if _, ok := e2eStackCluster(stack); ok {
				errs = multierr.Append(errs, s.unblockRoles(ctx, names[i]))
			}
		}
		_, err := s.cloudFormationClient.DeleteStack(ctx, &cloudformation.DeleteStackInput{
			StackName: lo.ToPtr(names[i]),
		})
		if err != nil {
			errs = multierr.Append(errs, err)
			continue
		}
		deleted = append(deleted, names[i])
	}
	return deleted, errs
}

// unblockRoles removes the policies from the stack's roles that failed to delete. A role can't be deleted while it has
// policies that the stack doesn't own, e.g. the PullThroughCachePolicy that private-cluster E2E adds to the eksctl node
// role. That leaves the nodegroup stack in DELETE_FAILED, which in turn blocks the cluster stack and the EKS cluster.
func (s *Stack) unblockRoles(ctx context.Context, stackName string) error {
	out, err := s.cloudFormationClient.DescribeStackResources(ctx, &cloudformation.DescribeStackResourcesInput{StackName: lo.ToPtr(stackName)})
	if err != nil {
		return err
	}
	var errs error
	for _, resource := range out.StackResources {
		if lo.FromPtr(resource.ResourceType) != "AWS::IAM::Role" || resource.ResourceStatus != cloudformationtypes.ResourceStatusDeleteFailed {
			continue
		}
		roleName := lo.FromPtr(resource.PhysicalResourceId)
		inline := iam.NewListRolePoliciesPaginator(s.iamClient, &iam.ListRolePoliciesInput{RoleName: lo.ToPtr(roleName)})
		for inline.HasMorePages() {
			page, err := inline.NextPage(ctx)
			if err != nil {
				errs = multierr.Append(errs, ignoreNoSuchEntity(err))
				break
			}
			for _, policyName := range page.PolicyNames {
				_, err := s.iamClient.DeleteRolePolicy(ctx, &iam.DeleteRolePolicyInput{RoleName: lo.ToPtr(roleName), PolicyName: lo.ToPtr(policyName)})
				errs = multierr.Append(errs, ignoreNoSuchEntity(err))
			}
		}
		attached := iam.NewListAttachedRolePoliciesPaginator(s.iamClient, &iam.ListAttachedRolePoliciesInput{RoleName: lo.ToPtr(roleName)})
		for attached.HasMorePages() {
			page, err := attached.NextPage(ctx)
			if err != nil {
				errs = multierr.Append(errs, ignoreNoSuchEntity(err))
				break
			}
			for _, policy := range page.AttachedPolicies {
				_, err := s.iamClient.DetachRolePolicy(ctx, &iam.DetachRolePolicyInput{RoleName: lo.ToPtr(roleName), PolicyArn: policy.PolicyArn})
				errs = multierr.Append(errs, ignoreNoSuchEntity(err))
			}
		}
	}
	return errs
}

func (s *Stack) getAllStacks(ctx context.Context) (stacks []cloudformationtypes.Stack, err error) {
	paginator := cloudformation.NewDescribeStacksPaginator(s.cloudFormationClient, &cloudformation.DescribeStacksInput{})

	for paginator.HasMorePages() {
		out, err := paginator.NextPage(ctx)
		if err != nil {
			return stacks, err
		}
		stacks = append(stacks, out.Stacks...)
	}

	return stacks, nil
}

func expiredStacks(stacks []cloudformationtypes.Stack, expirationTime time.Time, excludedClusters []string) (names []string) {
	activeStacks := lo.Reject(stacks, func(s cloudformationtypes.Stack, _ int) bool {
		return s.StackStatus == cloudformationtypes.StackStatusDeleteComplete ||
			s.StackStatus == cloudformationtypes.StackStatusDeleteInProgress
	})
	for _, stack := range activeStacks {
		if !lo.FromPtr(stack.CreationTime).Before(expirationTime) {
			continue
		}
		// Stacks left behind by a failed `aws cloudformation deploy` have no tags, so we match them by name
		if isFailedDeployStack(stack) {
			if !slices.Contains(excludedClusters, strings.TrimPrefix(lo.FromPtr(stack.StackName), iamStackPrefix)) {
				names = append(names, lo.FromPtr(stack.StackName))
			}
			continue
		}
		clusterName, found := lo.Find(stack.Tags, func(tag cloudformationtypes.Tag) bool {
			return lo.FromPtr(tag.Key) == karpenterTestingTag
		})
		if found && slices.Contains(excludedClusters, lo.FromPtr(clusterName.Value)) {
			continue
		}
		if _, found := lo.Find(stack.Tags, func(t cloudformationtypes.Tag) bool {
			return lo.FromPtr(t.Key) == karpenterTestingTag || lo.FromPtr(t.Key) == githubRunURLTag
		}); found {
			names = append(names, lo.FromPtr(stack.StackName))
		}
	}
	return names
}

func clusterStacks(stacks []cloudformationtypes.Stack, clusterName string) (names []string) {
	for _, stack := range stacks {
		if isFailedDeployStack(stack) && lo.FromPtr(stack.StackName) == iamStackPrefix+clusterName {
			names = append(names, lo.FromPtr(stack.StackName))
			continue
		}
		if _, found := lo.Find(stack.Tags, func(t cloudformationtypes.Tag) bool {
			return lo.FromPtr(t.Key) == karpenterTestingTag && lo.FromPtr(t.Value) == clusterName
		}); found {
			names = append(names, lo.FromPtr(stack.StackName))
		}
	}
	return names
}

func isFailedDeployStack(stack cloudformationtypes.Stack) bool {
	return stack.StackStatus == cloudformationtypes.StackStatusReviewInProgress &&
		strings.HasPrefix(lo.FromPtr(stack.StackName), iamStackPrefix)
}

// e2eStackCluster returns the E2E cluster a stack belongs to, if it is an eksctl-<cluster>-* or iam-<cluster> stack
func e2eStackCluster(stack cloudformationtypes.Stack) (string, bool) {
	name := lo.FromPtr(stack.StackName)
	cluster := strings.TrimPrefix(name, iamStackPrefix)
	if tag, found := lo.Find(stack.Tags, func(t cloudformationtypes.Tag) bool { return lo.FromPtr(t.Key) == karpenterTestingTag }); found {
		cluster = lo.FromPtr(tag.Value)
	}
	if name != iamStackPrefix+cluster && !strings.HasPrefix(name, eksctlStackPrefix+cluster+"-") {
		return "", false
	}
	return cluster, isE2ECluster(cluster)
}

func ignoreNoSuchEntity(err error) error {
	var noSuchEntity *iamtypes.NoSuchEntityException
	if errors.As(err, &noSuchEntity) {
		return nil
	}
	return err
}
