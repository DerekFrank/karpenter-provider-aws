#!/usr/bin/env bash

ROLE_ARN="arn:aws:iam::$ACCOUNT_ID:role/KarpenterNodeRole-$CLUSTER_NAME"

# Private clusters run this from CodeBuild, whose role can't call eks:DescribeClusterVersions (eksctl >= v0.201.0
# calls it for every command), so edit aws-auth directly with the cluster admin access entry CodeBuild already has
if [[ "$PRIVATE_CLUSTER" == 'true' ]]; then
  MAP_ROLES="$(kubectl get configmap aws-auth -n kube-system -o jsonpath='{.data.mapRoles}' 2>/dev/null)"
  MAP_ROLES+="
- rolearn: $ROLE_ARN
  username: system:node:{{EC2PrivateDNSName}}
  groups:
  - system:bootstrappers
  - system:nodes
  - eks:kube-proxy-windows"
  kubectl create configmap aws-auth -n kube-system --from-literal=mapRoles="$MAP_ROLES" --dry-run=client -o yaml | kubectl apply -f -
  exit $?
fi

eksctl create iamidentitymapping \
--username system:node:{{EC2PrivateDNSName}} \
--cluster "$CLUSTER_NAME" \
--arn "$ROLE_ARN" \
--group system:bootstrappers \
--group system:nodes \
--group eks:kube-proxy-windows
