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

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/samber/lo"
	"go.uber.org/multierr"
	v1 "k8s.io/api/core/v1"
)

// nodeRolePrefix is the name prefix of the node role that setup-cluster's iam-<cluster> stack creates.
// A Karpenter-created instance profile still holding this role blocks the stack from deleting the role.
const nodeRolePrefix = "KarpenterNodeRole-"

type InstanceProfile struct {
	iamClient *iam.Client
	clusters  *clusterChecker
}

func NewInstanceProfile(iamClient *iam.Client, eksClient *eks.Client) *InstanceProfile {
	return &InstanceProfile{iamClient: iamClient, clusters: newClusterChecker(eksClient)}
}

func (ip *InstanceProfile) String() string {
	return "InstanceProfile"
}

func (ip *InstanceProfile) Global() bool {
	return true
}

func (ip *InstanceProfile) GetExpired(ctx context.Context, expirationTime time.Time, excludedClusters []string) (names []string, err error) {
	instanceProfiles, err := ip.getAllInstanceProfiles(ctx)
	if err != nil {
		return names, err
	}
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return names, err
	}

	// Since we can only get the date of the instance profile (not the exact time the instance profile was created)
	// we add a day to the time that it was created to account for the worst-case of the instance profile being created
	// at 23:59:59 and being marked with a time of 00:00:00 due to only capturing the date and not the time.
	// We filter on this before listing tags, since ListInstanceProfileTags is called once per profile
	instanceProfiles = lo.Filter(instanceProfiles, func(profile iamtypes.InstanceProfile, _ int) bool {
		return lo.FromPtr(profile.CreateDate).Add(time.Hour * 24).Before(expirationTime)
	})

	errs := make([]error, len(instanceProfiles))
	for i := range instanceProfiles {
		profiles, err := ip.iamClient.ListInstanceProfileTags(ctx, &iam.ListInstanceProfileTagsInput{
			InstanceProfileName: instanceProfiles[i].InstanceProfileName,
		})
		if err != nil {
			errs[i] = err
			continue
		}

		// Karpenter-created profiles don't carry the testing tag, so they're matched by the node role they still hold
		if cluster, ok := nodeRoleProfileCluster(instanceProfiles[i], profiles.Tags, cfg.Region); ok {
			if slices.Contains(excludedClusters, cluster) || !isE2ECluster(cluster) {
				continue
			}
			gone, err := ip.clusters.Gone(ctx, cluster)
			if err != nil {
				errs[i] = err
				continue
			}
			if gone {
				names = append(names, lo.FromPtr(instanceProfiles[i].InstanceProfileName))
			}
			continue
		}

		clusterName, _ := lo.Find(profiles.Tags, func(tag iamtypes.Tag) bool {
			return lo.FromPtr(tag.Key) == karpenterTestingTag
		})
		// Checking to make sure we are only list resources in the given region
		region, _ := lo.Find(profiles.Tags, func(tag iamtypes.Tag) bool {
			return lo.FromPtr(tag.Key) == v1.LabelTopologyRegion
		})
		if slices.Contains(excludedClusters, lo.FromPtr(clusterName.Value)) || lo.FromPtr(region.Value) != cfg.Region {
			continue
		}
		if lo.FromPtr(clusterName.Value) != "" {
			names = append(names, lo.FromPtr(instanceProfiles[i].InstanceProfileName))
		}
	}

	return names, multierr.Combine(errs...)
}

func (ip *InstanceProfile) CountAll(ctx context.Context) (count int, err error) {
	instanceProfiles, err := ip.getAllInstanceProfiles(ctx)
	if err != nil {
		return count, err
	}
	return len(instanceProfiles), err
}

func (ip *InstanceProfile) Get(ctx context.Context, clusterName string) (names []string, err error) {
	instanceProfiles, err := ip.getAllInstanceProfiles(ctx)
	if err != nil {
		return names, err
	}
	// Only list tags for the profiles Karpenter would have created for this cluster, rather than every profile in the account
	instanceProfiles = clusterInstanceProfiles(instanceProfiles, clusterName)

	errs := make([]error, len(instanceProfiles))
	for i := range instanceProfiles {
		profiles, err := ip.iamClient.ListInstanceProfileTags(ctx, &iam.ListInstanceProfileTagsInput{
			InstanceProfileName: instanceProfiles[i].InstanceProfileName,
		})
		if err != nil {
			errs[i] = err
			continue
		}

		if _, found := lo.Find(profiles.Tags, func(t iamtypes.Tag) bool {
			return lo.FromPtr(t.Key) == karpenterTestingTag && lo.FromPtr(t.Value) == clusterName
		}); found {
			names = append(names, lo.FromPtr(instanceProfiles[i].InstanceProfileName))
			continue
		}
		// Karpenter-created profiles don't carry the testing tag. Only delete them once the cluster is gone
		if _, found := lo.Find(profiles.Tags, func(t iamtypes.Tag) bool {
			return lo.FromPtr(t.Key) == karpenterClusterNameTag && lo.FromPtr(t.Value) == clusterName
		}); found {
			gone, err := ip.clusters.Gone(ctx, clusterName)
			if err != nil {
				errs[i] = err
				continue
			}
			if gone {
				names = append(names, lo.FromPtr(instanceProfiles[i].InstanceProfileName))
			}
		}
	}

	return names, multierr.Combine(errs...)
}

// Cleanup any old instance profiles that were managed by Karpenter or were provisioned as part of testing
// We execute these in serial since we will most likely get rate limited if we try to delete these too aggressively
func (ip *InstanceProfile) Cleanup(ctx context.Context, names []string) ([]string, error) {
	var deleted []string
	var errs error
	for i := range names {
		out, err := ip.iamClient.GetInstanceProfile(ctx, &iam.GetInstanceProfileInput{InstanceProfileName: lo.ToPtr(names[i])})
		if err != nil {
			errs = multierr.Append(errs, err)
			continue
		}
		for _, role := range out.InstanceProfile.Roles {
			if _, err := ip.iamClient.RemoveRoleFromInstanceProfile(ctx, &iam.RemoveRoleFromInstanceProfileInput{
				InstanceProfileName: lo.ToPtr(names[i]),
				RoleName:            role.RoleName,
			}); err != nil {
				errs = multierr.Append(errs, err)
			}
		}
		if _, err := ip.iamClient.DeleteInstanceProfile(ctx, &iam.DeleteInstanceProfileInput{
			InstanceProfileName: lo.ToPtr(names[i]),
		}); err != nil {
			errs = multierr.Append(errs, err)
			continue
		}
		deleted = append(deleted, names[i])
	}
	return deleted, errs
}

func (ip *InstanceProfile) getAllInstanceProfiles(ctx context.Context) (instanceprofiles []iamtypes.InstanceProfile, err error) {
	paginator := iam.NewListInstanceProfilesPaginator(ip.iamClient, &iam.ListInstanceProfilesInput{})

	for paginator.HasMorePages() {
		out, err := paginator.NextPage(ctx)
		if err != nil {
			return instanceprofiles, err
		}
		instanceprofiles = append(instanceprofiles, out.InstanceProfiles...)
	}

	return instanceprofiles, nil
}

// clusterInstanceProfiles returns the profiles that are named the way Karpenter names the instance profiles it creates
// for a cluster ("<cluster-name>_<hash>", see EC2NodeClass.InstanceProfileName)
func clusterInstanceProfiles(instanceProfiles []iamtypes.InstanceProfile, clusterName string) []iamtypes.InstanceProfile {
	return lo.Filter(instanceProfiles, func(profile iamtypes.InstanceProfile, _ int) bool {
		return strings.HasPrefix(lo.FromPtr(profile.InstanceProfileName), clusterName+"_")
	})
}

// nodeRoleProfileCluster returns the cluster of a Karpenter-created instance profile in region that still holds the
// cluster's KarpenterNodeRole-<cluster> role
func nodeRoleProfileCluster(profile iamtypes.InstanceProfile, tags []iamtypes.Tag, region string) (string, bool) {
	clusterTag, found := lo.Find(tags, func(t iamtypes.Tag) bool { return lo.FromPtr(t.Key) == karpenterClusterNameTag })
	if !found || lo.FromPtr(clusterTag.Value) == "" {
		return "", false
	}
	cluster := lo.FromPtr(clusterTag.Value)
	if _, found := lo.Find(tags, func(t iamtypes.Tag) bool {
		return lo.FromPtr(t.Key) == v1.LabelTopologyRegion && lo.FromPtr(t.Value) == region
	}); !found {
		return "", false
	}
	if !strings.HasPrefix(lo.FromPtr(profile.InstanceProfileName), cluster+"_") {
		return "", false
	}
	if _, found := lo.Find(profile.Roles, func(r iamtypes.Role) bool { return lo.FromPtr(r.RoleName) == nodeRolePrefix+cluster }); !found {
		return "", false
	}
	return cluster, true
}
