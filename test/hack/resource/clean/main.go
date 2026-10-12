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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/aws/karpenter-provider-aws/test/hack/resource/pkg/metrics"
	"github.com/aws/karpenter-provider-aws/test/hack/resource/pkg/resourcetypes"
)

const sweeperCleanedResourcesTableName = "sweeperCleanedResources"

var excludedClusters = []string{}

func main() {
	expiration := flag.String("expiration", "12h", "define the expirationTTL of the resources")
	clusterName := flag.String("cluster-name", "", "define cluster name to cleanup")
	flag.Parse()

	ctx := context.Background()
	cfg := lo.Must(config.LoadDefaultConfig(ctx))
	cfg.RetryMaxAttempts = 10

	logger := lo.Must(zap.NewProduction()).Sugar()

	expirationTTL, err := time.ParseDuration(lo.FromPtr(expiration))
	if err != nil {
		logger.Fatalln("need a valid expiration duration", err)
	}
	expirationTime := time.Now().Add(-expirationTTL)

	logger.With("expiration-time", expirationTime.String()).Infof("resolved expiration time for all resourceTypes")

	ec2Client := ec2.NewFromConfig(cfg)
	cloudFormationClient := cloudformation.NewFromConfig(cfg)
	iamClient := iam.NewFromConfig(cfg)
	eksClient := eks.NewFromConfig(cfg)

	metricsClient := metrics.Client(metrics.NewTimeStream(cfg))

	// These resources are intentionally ordered so that instances that are using ENIs
	// will be cleaned before ENIs are attempted to be cleaned up. Likewise, instances and ENIs
	// are cleaned up before security groups are cleaned up to ensure that everything is detached and doesn't
	// prevent deletion. ENIs and instance profiles are cleaned before stacks, since leaked branch ENIs block the
	// eksctl stack from deleting its subnets and security groups, and instance profiles holding the node role block
	// the iam-<cluster> stack from deleting the role
	resourceTypes := []resourcetypes.Type{
		resourcetypes.NewInstance(ec2Client),
		resourcetypes.NewCapacityReservation(ec2Client),
		resourcetypes.NewVPCEndpoint(ec2Client),
		resourcetypes.NewENI(ec2Client),
		resourcetypes.NewSecurityGroup(ec2Client, eksClient),
		resourcetypes.NewLaunchTemplate(ec2Client),
		resourcetypes.NewOIDC(iamClient),
		resourcetypes.NewInstanceProfile(iamClient, eksClient),
		resourcetypes.NewStack(cloudFormationClient, iamClient),
		resourcetypes.NewVPCPeeringConnection(ec2Client),
	}

	sweep := lo.FromPtr(clusterName) == ""
	var failed []string
	for i := range resourceTypes {
		resourceLogger := logger.With("type", resourceTypes[i].String())
		var ids []string
		var err error
		var typeFailed bool
		// If there's no cluster defined, clean up all expired resources. otherwise, only cleanup the resources associated with the cluster
		if sweep {
			ids, err = resourceTypes[i].GetExpired(ctx, expirationTime, excludedClusters)
		} else if !slices.Contains(excludedClusters, lo.FromPtr(clusterName)) {
			ids, err = resourceTypes[i].Get(ctx, lo.FromPtr(clusterName))
		}
		if err != nil {
			resourceLogger.Errorf("%v", err)
			typeFailed = true
		}
		cleaned := []string{}
		resourceLogger.With("ids", ids, "count", len(ids)).Infof("discovered resourceTypes")
		if len(ids) > 0 {
			cleaned, err = resourceTypes[i].Cleanup(ctx, ids)
			if err != nil {
				resourceLogger.Errorf("%v", err)
				typeFailed = true
			}
			resourceLogger.With("ids", cleaned, "count", len(cleaned)).Infof("deleted resourceTypes")
		}
		// Should only fire metrics if the resource have expired
		if sweep {
			if err = metricsClient.FireMetric(ctx, sweeperCleanedResourcesTableName, fmt.Sprintf("%sDeleted", resourceTypes[i].String()), float64(len(cleaned)), lo.Ternary(resourceTypes[i].Global(), "global", cfg.Region)); err != nil {
				resourceLogger.Errorf("%v", err)
			}
		}
		resourceLogger.With("discovered", len(ids), "deleted", len(cleaned), "failed", typeFailed).Infof("summary")
		if typeFailed {
			failed = append(failed, resourceTypes[i].String())
		}
	}
	// The sweeper is the backstop for leaks, so it fails loudly. Per-cluster cleanup runs at the end of every E2E job
	// and only logs, so that a cleanup error doesn't fail a suite that passed
	if len(failed) > 0 {
		logger.With("types", failed).Errorf("failed to clean up some resources")
		if sweep {
			os.Exit(1)
		}
	}
}
