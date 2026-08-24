---
title: "Metrics"
linkTitle: "Metrics"
weight: 7

description: >
  Inspect Karpenter Metrics
---
<!-- this document is generated from hack/docs/metrics_gen/main.go -->
Karpenter makes several metrics available in Prometheus format to allow monitoring cluster provisioning status. These metrics are available by default at `karpenter.kube-system.svc.cluster.local:8080/metrics` configurable via the `METRICS_PORT` environment variable documented [here](../settings)
### `karpenter_consolidation_score`
Score of balanced consolidation moves. Labeled by decision, NodePool, and policy.
- Stability Level: ALPHA
- Dimensions:
  - `decision` — The disruption decision taken for the candidate(s).
    - `no-op` — No disruption action was taken.
    - `replace` — The candidate(s) were replaced with more efficient capacity.
    - `delete` — The candidate(s) were deleted without replacement.
    - `approved` — The disruption decision was approved for execution.
    - `rejected` — The disruption decision was rejected before execution.
  - `nodepool` — The name of the NodePool that owns the resource.
  - `policy` — The NodePool consolidation policy in effect for the move.

### `karpenter_consolidation_moves_total`
Number of balanced consolidation moves. Labeled by decision, NodePool, and policy.
- Stability Level: ALPHA
- Dimensions:
  - `decision` — The disruption decision taken for the candidate(s).
    - `no-op` — No disruption action was taken.
    - `replace` — The candidate(s) were replaced with more efficient capacity.
    - `delete` — The candidate(s) were deleted without replacement.
    - `approved` — The disruption decision was approved for execution.
    - `rejected` — The disruption decision was rejected before execution.
  - `nodepool` — The name of the NodePool that owns the resource.
  - `policy` — The NodePool consolidation policy in effect for the move.

### `karpenter_build_info`
A metric with a constant '1' value labeled by version from which karpenter was built.
- Stability Level: STABLE
- Dimensions:
  - `version`
  - `goversion`
  - `goarch`
  - `commit`

## Nodeclaims Metrics

### `karpenter_nodeclaims_unhealthy_disrupted_total`
Number of unhealthy nodeclaims disrupted in total by Karpenter. Labeled by condition on the node was disrupted, the owning nodepool, and the image ID.
- Stability Level: ALPHA
- Dimensions:
  - `condition`
  - `nodepool` — The name of the NodePool that owns the resource.
  - `capacity_type` — The capacity type of the instance.
    - `on-demand` — On-demand capacity.
    - `spot` — Spot capacity, which can be reclaimed by the cloud provider.
    - `reserved` — Reserved capacity, backed by a capacity reservation.
  - `image_id`

### `karpenter_nodeclaims_termination_duration_seconds`
Duration of NodeClaim termination in seconds.
- Stability Level: BETA
- Dimensions:
  - `nodepool` — The name of the NodePool that owns the resource.

### `karpenter_nodeclaims_terminated_total`
Number of nodeclaims terminated in total by Karpenter. Labeled by the owning nodepool, capacity type, and zone.
- Stability Level: STABLE
- Dimensions:
  - `nodepool` — The name of the NodePool that owns the resource.
  - `capacity_type` — The capacity type of the instance.
    - `on-demand` — On-demand capacity.
    - `spot` — Spot capacity, which can be reclaimed by the cloud provider.
    - `reserved` — Reserved capacity, backed by a capacity reservation.
  - `zone` — The availability zone of the instance.

### `karpenter_nodeclaims_instance_termination_duration_seconds`
Duration of CloudProvider Instance termination in seconds.
- Stability Level: BETA
- Dimensions:
  - `nodepool` — The name of the NodePool that owns the resource.

### `karpenter_nodeclaims_disrupted_total`
Number of nodeclaims disrupted in total by Karpenter. Labeled by reason the nodeclaim was disrupted, the owning nodepool, the capacity type, the consolidation policy, and the termination mode.
- Stability Level: ALPHA
- Dimensions:
  - `reason` — Why the action was taken. Values are metric-specific: create/delete counters use `provisioned`, `expired`, or `unhealthy`; disruption metrics use the disruption reason such as `underutilized`, `empty`, `drifted`, or `expired`; cloud-provider failure metrics use the provider error reason.
  - `nodepool` — The name of the NodePool that owns the resource.
  - `capacity_type` — The capacity type of the instance.
    - `on-demand` — On-demand capacity.
    - `spot` — Spot capacity, which can be reclaimed by the cloud provider.
    - `reserved` — Reserved capacity, backed by a capacity reservation.
  - `consolidation_policy` — The NodePool consolidation policy in effect.
    - `WhenEmpty` — Consolidate only nodes with no workload pods.
    - `WhenEmptyOrUnderutilized` — Consolidate empty nodes and underutilized nodes.
    - `Balanced` — Consolidate using the balanced algorithm.
  - `termination_mode` — The termination mode used to disrupt the node.
    - `graceful` — Graceful termination that respects the node's disruption budget and drains pods.
    - `eventual` — Eventual termination once the node's terminationGracePeriod elapses.
    - `forceful` — Forceful termination that deletes the node immediately.

### `karpenter_nodeclaims_created_total`
Number of nodeclaims created in total by Karpenter. Labeled by reason the nodeclaim was created, the owning nodepool, and if min values was relaxed for this nodeclaim.
- Stability Level: STABLE
- Dimensions:
  - `reason` — Why the action was taken. Values are metric-specific: create/delete counters use `provisioned`, `expired`, or `unhealthy`; disruption metrics use the disruption reason such as `underutilized`, `empty`, `drifted`, or `expired`; cloud-provider failure metrics use the provider error reason.
  - `nodepool` — The name of the NodePool that owns the resource.
  - `min_values_relaxed` — Whether minValues requirements were relaxed to satisfy scheduling.

## Nodeclaim Termination Metrics

### `operator_nodeclaim_termination_duration_seconds`
The amount of time taken by an object to terminate completely.
- Stability Level: BETA

### `operator_nodeclaim_termination_current_time_seconds`
The current amount of time in seconds that an object has been in terminating state.
- Stability Level: BETA
- Dimensions:
  - `namespace` — The namespace of the object the metric describes.
  - `name` — The name of the object the metric describes.

## Nodeclaim Status Condition Metrics

### `operator_nodeclaim_status_condition_transitions_total`
The count of transitions of a given object, type and status.
- Stability Level: BETA
- Dimensions:
  - `type` — The type dimension. For status-condition metrics it is the status condition type (e.g. `Ready`); for event metrics it is the Kubernetes event type (`Normal` or `Warning`).
    - `Launched` — The instance backing the NodeClaim has been launched with the cloud provider.
    - `Registered` — The launched node has registered with the cluster.
    - `Initialized` — The registered node has finished initializing and is ready for workloads.
    - `Consolidatable` — The NodeClaim is currently eligible for consolidation.
    - `Drifted` — The NodeClaim has drifted from its desired specification.
    - `Drained` — The node has been drained of pods during termination.
    - `VolumesDetached` — The node's volumes have been detached during termination.
    - `InstanceTerminating` — The backing instance is terminating.
    - `ConsistentStateFound` — A consistent instance state was observed before disruption.
    - `DisruptionReason` — Set while the NodeClaim is being disrupted; records the disruption reason.
  - `status` — The status of a status condition (e.g. the `Ready` condition). For transition metrics this is the state being left.
  - `reason` — The reason dimension. For status-condition metrics it is the condition reason; for event metrics it is the Kubernetes event reason.

### `operator_nodeclaim_status_condition_transition_seconds`
The amount of time a condition was in a given state before transitioning. e.g. Alarm := P99(Updated=False) > 5 minutes
- Stability Level: BETA
- Dimensions:
  - `type` — The type dimension. For status-condition metrics it is the status condition type (e.g. `Ready`); for event metrics it is the Kubernetes event type (`Normal` or `Warning`).
    - `Launched` — The instance backing the NodeClaim has been launched with the cloud provider.
    - `Registered` — The launched node has registered with the cluster.
    - `Initialized` — The registered node has finished initializing and is ready for workloads.
    - `Consolidatable` — The NodeClaim is currently eligible for consolidation.
    - `Drifted` — The NodeClaim has drifted from its desired specification.
    - `Drained` — The node has been drained of pods during termination.
    - `VolumesDetached` — The node's volumes have been detached during termination.
    - `InstanceTerminating` — The backing instance is terminating.
    - `ConsistentStateFound` — A consistent instance state was observed before disruption.
    - `DisruptionReason` — Set while the NodeClaim is being disrupted; records the disruption reason.
  - `status` — The status of a status condition (e.g. the `Ready` condition). For transition metrics this is the state being left.
  - `to_status` — The status a condition transitioned to, for transition metrics.

### `operator_nodeclaim_status_condition_current_status_seconds`
The current amount of time in seconds that a status condition has been in a specific state. Alarm := P99(Updated=Unknown) > 5 minutes
- Stability Level: BETA
- Dimensions:
  - `namespace` — The namespace of the object the metric describes.
  - `name` — The name of the object the metric describes.
  - `type` — The type dimension. For status-condition metrics it is the status condition type (e.g. `Ready`); for event metrics it is the Kubernetes event type (`Normal` or `Warning`).
    - `Launched` — The instance backing the NodeClaim has been launched with the cloud provider.
    - `Registered` — The launched node has registered with the cluster.
    - `Initialized` — The registered node has finished initializing and is ready for workloads.
    - `Consolidatable` — The NodeClaim is currently eligible for consolidation.
    - `Drifted` — The NodeClaim has drifted from its desired specification.
    - `Drained` — The node has been drained of pods during termination.
    - `VolumesDetached` — The node's volumes have been detached during termination.
    - `InstanceTerminating` — The backing instance is terminating.
    - `ConsistentStateFound` — A consistent instance state was observed before disruption.
    - `DisruptionReason` — Set while the NodeClaim is being disrupted; records the disruption reason.
  - `status` — The status of a status condition (e.g. the `Ready` condition). For transition metrics this is the state being left.
  - `reason` — The reason dimension. For status-condition metrics it is the condition reason; for event metrics it is the Kubernetes event reason.

### `operator_nodeclaim_status_condition_count`
The number of a condition for a given object, type and status. e.g. Alarm := Available=False > 0
- Stability Level: BETA
- Dimensions:
  - `namespace` — The namespace of the object the metric describes.
  - `name` — The name of the object the metric describes.
  - `type` — The type dimension. For status-condition metrics it is the status condition type (e.g. `Ready`); for event metrics it is the Kubernetes event type (`Normal` or `Warning`).
    - `Launched` — The instance backing the NodeClaim has been launched with the cloud provider.
    - `Registered` — The launched node has registered with the cluster.
    - `Initialized` — The registered node has finished initializing and is ready for workloads.
    - `Consolidatable` — The NodeClaim is currently eligible for consolidation.
    - `Drifted` — The NodeClaim has drifted from its desired specification.
    - `Drained` — The node has been drained of pods during termination.
    - `VolumesDetached` — The node's volumes have been detached during termination.
    - `InstanceTerminating` — The backing instance is terminating.
    - `ConsistentStateFound` — A consistent instance state was observed before disruption.
    - `DisruptionReason` — Set while the NodeClaim is being disrupted; records the disruption reason.
  - `status` — The status of a status condition (e.g. the `Ready` condition). For transition metrics this is the state being left.
  - `reason` — The reason dimension. For status-condition metrics it is the condition reason; for event metrics it is the Kubernetes event reason.

## Nodes Metrics

### `karpenter_nodes_total_pod_requests`
Node total pod requests are the resources requested by pods bound to nodes, including the DaemonSet pods.
- Stability Level: BETA

### `karpenter_nodes_total_pod_limits`
Node total pod limits are the resources specified by pod limits, including the DaemonSet pods.
- Stability Level: BETA

### `karpenter_nodes_total_daemon_requests`
Node total daemon requests are the resource requested by DaemonSet pods bound to nodes.
- Stability Level: BETA

### `karpenter_nodes_total_daemon_limits`
Node total daemon limits are the resources specified by DaemonSet pod limits.
- Stability Level: BETA

### `karpenter_nodes_termination_duration_seconds`
The time taken between a node's deletion request and the removal of its finalizer
- Stability Level: BETA
- Dimensions:
  - `nodepool` — The name of the NodePool that owns the resource.

### `karpenter_nodes_terminated_total`
Number of nodes terminated in total by Karpenter. Labeled by owning nodepool and zone.
- Stability Level: STABLE
- Dimensions:
  - `nodepool` — The name of the NodePool that owns the resource.
  - `zone` — The availability zone of the instance.

### `karpenter_nodes_system_overhead`
Node system daemon overhead are the resources reserved for system overhead, the difference between the node's capacity and allocatable values are reported by the status.
- Stability Level: BETA

### `karpenter_nodes_lifetime_duration_seconds`
The lifetime duration of the nodes since creation.
- Stability Level: ALPHA
- Dimensions:
  - `nodepool` — The name of the NodePool that owns the resource.

### `karpenter_nodes_drained_total`
The total number of nodes drained by Karpenter
- Stability Level: ALPHA
- Dimensions:
  - `nodepool` — The name of the NodePool that owns the resource.

### `karpenter_nodes_current_lifetime_seconds`
Node age in seconds
- Stability Level: ALPHA

### `karpenter_nodes_created_total`
Number of nodes created in total by Karpenter. Labeled by owning nodepool and zone.
- Stability Level: STABLE
- Dimensions:
  - `nodepool` — The name of the NodePool that owns the resource.
  - `zone` — The availability zone of the instance.

### `karpenter_nodes_allocatable`
Node allocatable are the resources allocatable by nodes.
- Stability Level: BETA

## Pods Metrics

### `karpenter_pods_unstarted_time_seconds`
The time from pod creation until the pod is running.
- Stability Level: ALPHA
- Dimensions:
  - `name`
  - `namespace`

### `karpenter_pods_unbound_time_seconds`
The time from pod creation until the pod is bound.
- Stability Level: ALPHA
- Dimensions:
  - `name`
  - `namespace`

### `karpenter_pods_state`
Pod state is the current state of pods. This metric can be used several ways as it is labeled by the pod name, namespace, owner, node, nodepool name, zone, architecture, capacity type, instance type, pod phase, and pod readiness.
- Stability Level: BETA

### `karpenter_pods_startup_duration_seconds`
The time from pod creation until the pod is running.
- Stability Level: STABLE

### `karpenter_pods_scheduling_decision_duration_seconds`
The time it takes for Karpenter to first try to schedule a pod after it's been seen.
- Stability Level: ALPHA

### `karpenter_pods_provisioning_unstarted_time_seconds`
The time from when Karpenter first thinks the pod can schedule until the pod is running. Note: this calculated from a point in memory, not by the pod creation timestamp.
- Stability Level: ALPHA
- Dimensions:
  - `name`
  - `namespace`

### `karpenter_pods_provisioning_unbound_time_seconds`
The time from when Karpenter first thinks the pod can schedule until it binds. Note: this calculated from a point in memory, not by the pod creation timestamp.
- Stability Level: ALPHA
- Dimensions:
  - `name`
  - `namespace`

### `karpenter_pods_provisioning_startup_duration_seconds`
The time from when Karpenter first thinks the pod can schedule until the pod is running. Note: this calculated from a point in memory, not by the pod creation timestamp.
- Stability Level: ALPHA

### `karpenter_pods_provisioning_scheduling_undecided_time_seconds`
The time from when Karpenter has seen a pod without making a scheduling decision for the pod. Note: this calculated from a point in memory, not by the pod creation timestamp.
- Stability Level: ALPHA
- Dimensions:
  - `name`
  - `namespace`

### `karpenter_pods_provisioning_bound_duration_seconds`
The time from when Karpenter first thinks the pod can schedule until it binds. Note: this calculated from a point in memory, not by the pod creation timestamp.
- Stability Level: ALPHA

### `karpenter_pods_eviction_requests_total`
The total number of pod eviction requests made by Karpenter, labeled by response code
- Stability Level: ALPHA
- Dimensions:
  - `code` — The HTTP response code returned by the Kubernetes eviction API for the eviction request.

### `karpenter_pods_drained_total`
The total number of pods drained during node termination by Karpenter, labeled by reason
- Stability Level: ALPHA
- Dimensions:
  - `reason` — Why the action was taken. Values are metric-specific: create/delete counters use `provisioned`, `expired`, or `unhealthy`; disruption metrics use the disruption reason such as `underutilized`, `empty`, `drifted`, or `expired`; cloud-provider failure metrics use the provider error reason.

### `karpenter_pods_disruption_initiated_total`
Number of pod disruptions initiated in total by Karpenter, incremented by the reschedulable pod count whenever the underlying nodeclaim is disrupted. Labeled by reason the nodeclaim was disrupted, the owning nodepool, the capacity type, the consolidation policy, and the termination mode. Pods owned by DaemonSets and mirror pods are excluded.
- Stability Level: ALPHA
- Dimensions:
  - `reason` — Why the action was taken. Values are metric-specific: create/delete counters use `provisioned`, `expired`, or `unhealthy`; disruption metrics use the disruption reason such as `underutilized`, `empty`, `drifted`, or `expired`; cloud-provider failure metrics use the provider error reason.
  - `nodepool` — The name of the NodePool that owns the resource.
  - `capacity_type` — The capacity type of the instance.
    - `on-demand` — On-demand capacity.
    - `spot` — Spot capacity, which can be reclaimed by the cloud provider.
    - `reserved` — Reserved capacity, backed by a capacity reservation.
  - `consolidation_policy` — The NodePool consolidation policy in effect.
    - `WhenEmpty` — Consolidate only nodes with no workload pods.
    - `WhenEmptyOrUnderutilized` — Consolidate empty nodes and underutilized nodes.
    - `Balanced` — Consolidate using the balanced algorithm.
  - `termination_mode` — The termination mode used to disrupt the node.
    - `graceful` — Graceful termination that respects the node's disruption budget and drains pods.
    - `eventual` — Eventual termination once the node's terminationGracePeriod elapses.
    - `forceful` — Forceful termination that deletes the node immediately.

### `karpenter_pods_bound_duration_seconds`
The time from pod creation until the pod is bound.
- Stability Level: ALPHA

## Nodepool Termination Metrics

### `operator_nodepool_termination_duration_seconds`
The amount of time taken by an object to terminate completely.
- Stability Level: BETA

### `operator_nodepool_termination_current_time_seconds`
The current amount of time in seconds that an object has been in terminating state.
- Stability Level: BETA
- Dimensions:
  - `namespace` — The namespace of the object the metric describes.
  - `name` — The name of the object the metric describes.

## Nodepool Status Condition Metrics

### `operator_nodepool_status_condition_transitions_total`
The count of transitions of a given object, type and status.
- Stability Level: BETA
- Dimensions:
  - `type` — The type dimension. For status-condition metrics it is the status condition type (e.g. `Ready`); for event metrics it is the Kubernetes event type (`Normal` or `Warning`).
    - `ValidationSucceeded` — The runtime-based configuration is valid for this NodePool.
    - `NodeClassReady` — The underlying NodeClass was resolved and is reporting as Ready.
    - `NodeRegistrationHealthy` — No misconfiguration is preventing successful node launch/registration.
  - `status` — The status of a status condition (e.g. the `Ready` condition). For transition metrics this is the state being left.
  - `reason` — The reason dimension. For status-condition metrics it is the condition reason; for event metrics it is the Kubernetes event reason.

### `operator_nodepool_status_condition_transition_seconds`
The amount of time a condition was in a given state before transitioning. e.g. Alarm := P99(Updated=False) > 5 minutes
- Stability Level: BETA
- Dimensions:
  - `type` — The type dimension. For status-condition metrics it is the status condition type (e.g. `Ready`); for event metrics it is the Kubernetes event type (`Normal` or `Warning`).
    - `ValidationSucceeded` — The runtime-based configuration is valid for this NodePool.
    - `NodeClassReady` — The underlying NodeClass was resolved and is reporting as Ready.
    - `NodeRegistrationHealthy` — No misconfiguration is preventing successful node launch/registration.
  - `status` — The status of a status condition (e.g. the `Ready` condition). For transition metrics this is the state being left.
  - `to_status` — The status a condition transitioned to, for transition metrics.

### `operator_nodepool_status_condition_current_status_seconds`
The current amount of time in seconds that a status condition has been in a specific state. Alarm := P99(Updated=Unknown) > 5 minutes
- Stability Level: BETA
- Dimensions:
  - `namespace` — The namespace of the object the metric describes.
  - `name` — The name of the object the metric describes.
  - `type` — The type dimension. For status-condition metrics it is the status condition type (e.g. `Ready`); for event metrics it is the Kubernetes event type (`Normal` or `Warning`).
    - `ValidationSucceeded` — The runtime-based configuration is valid for this NodePool.
    - `NodeClassReady` — The underlying NodeClass was resolved and is reporting as Ready.
    - `NodeRegistrationHealthy` — No misconfiguration is preventing successful node launch/registration.
  - `status` — The status of a status condition (e.g. the `Ready` condition). For transition metrics this is the state being left.
  - `reason` — The reason dimension. For status-condition metrics it is the condition reason; for event metrics it is the Kubernetes event reason.

### `operator_nodepool_status_condition_count`
The number of a condition for a given object, type and status. e.g. Alarm := Available=False > 0
- Stability Level: BETA
- Dimensions:
  - `namespace` — The namespace of the object the metric describes.
  - `name` — The name of the object the metric describes.
  - `type` — The type dimension. For status-condition metrics it is the status condition type (e.g. `Ready`); for event metrics it is the Kubernetes event type (`Normal` or `Warning`).
    - `ValidationSucceeded` — The runtime-based configuration is valid for this NodePool.
    - `NodeClassReady` — The underlying NodeClass was resolved and is reporting as Ready.
    - `NodeRegistrationHealthy` — No misconfiguration is preventing successful node launch/registration.
  - `status` — The status of a status condition (e.g. the `Ready` condition). For transition metrics this is the state being left.
  - `reason` — The reason dimension. For status-condition metrics it is the condition reason; for event metrics it is the Kubernetes event reason.

## Ec2nodeclass Termination Metrics

### `operator_ec2nodeclass_termination_duration_seconds`
The amount of time taken by an object to terminate completely.
- Stability Level: BETA

### `operator_ec2nodeclass_termination_current_time_seconds`
The current amount of time in seconds that an object has been in terminating state.
- Stability Level: BETA
- Dimensions:
  - `namespace` — The namespace of the object the metric describes.
  - `name` — The name of the object the metric describes.

## Ec2nodeclass Status Condition Metrics

### `operator_ec2nodeclass_status_condition_transitions_total`
The count of transitions of a given object, type and status.
- Stability Level: BETA
- Dimensions:
  - `type` — The type dimension. For status-condition metrics it is the status condition type (e.g. `Ready`); for event metrics it is the Kubernetes event type (`Normal` or `Warning`).
    - `SubnetsReady` — The selected subnets were resolved.
    - `SecurityGroupsReady` — The selected security groups were resolved.
    - `AMIsReady` — The selected AMIs were resolved.
    - `InstanceProfileReady` — The instance profile was resolved or created.
    - `CapacityReservationsReady` — The selected capacity reservations were resolved.
    - `ValidationSucceeded` — The EC2NodeClass configuration passed validation.
    - `PlacementGroupReady` — The selected placement group was resolved.
  - `status` — The status of a status condition (e.g. the `Ready` condition). For transition metrics this is the state being left.
  - `reason` — The reason dimension. For status-condition metrics it is the condition reason; for event metrics it is the Kubernetes event reason.

### `operator_ec2nodeclass_status_condition_transition_seconds`
The amount of time a condition was in a given state before transitioning. e.g. Alarm := P99(Updated=False) > 5 minutes
- Stability Level: BETA
- Dimensions:
  - `type` — The type dimension. For status-condition metrics it is the status condition type (e.g. `Ready`); for event metrics it is the Kubernetes event type (`Normal` or `Warning`).
    - `SubnetsReady` — The selected subnets were resolved.
    - `SecurityGroupsReady` — The selected security groups were resolved.
    - `AMIsReady` — The selected AMIs were resolved.
    - `InstanceProfileReady` — The instance profile was resolved or created.
    - `CapacityReservationsReady` — The selected capacity reservations were resolved.
    - `ValidationSucceeded` — The EC2NodeClass configuration passed validation.
    - `PlacementGroupReady` — The selected placement group was resolved.
  - `status` — The status of a status condition (e.g. the `Ready` condition). For transition metrics this is the state being left.
  - `to_status` — The status a condition transitioned to, for transition metrics.

### `operator_ec2nodeclass_status_condition_current_status_seconds`
The current amount of time in seconds that a status condition has been in a specific state. Alarm := P99(Updated=Unknown) > 5 minutes
- Stability Level: BETA
- Dimensions:
  - `namespace` — The namespace of the object the metric describes.
  - `name` — The name of the object the metric describes.
  - `type` — The type dimension. For status-condition metrics it is the status condition type (e.g. `Ready`); for event metrics it is the Kubernetes event type (`Normal` or `Warning`).
    - `SubnetsReady` — The selected subnets were resolved.
    - `SecurityGroupsReady` — The selected security groups were resolved.
    - `AMIsReady` — The selected AMIs were resolved.
    - `InstanceProfileReady` — The instance profile was resolved or created.
    - `CapacityReservationsReady` — The selected capacity reservations were resolved.
    - `ValidationSucceeded` — The EC2NodeClass configuration passed validation.
    - `PlacementGroupReady` — The selected placement group was resolved.
  - `status` — The status of a status condition (e.g. the `Ready` condition). For transition metrics this is the state being left.
  - `reason` — The reason dimension. For status-condition metrics it is the condition reason; for event metrics it is the Kubernetes event reason.

### `operator_ec2nodeclass_status_condition_count`
The number of a condition for a given object, type and status. e.g. Alarm := Available=False > 0
- Stability Level: BETA
- Dimensions:
  - `namespace` — The namespace of the object the metric describes.
  - `name` — The name of the object the metric describes.
  - `type` — The type dimension. For status-condition metrics it is the status condition type (e.g. `Ready`); for event metrics it is the Kubernetes event type (`Normal` or `Warning`).
    - `SubnetsReady` — The selected subnets were resolved.
    - `SecurityGroupsReady` — The selected security groups were resolved.
    - `AMIsReady` — The selected AMIs were resolved.
    - `InstanceProfileReady` — The instance profile was resolved or created.
    - `CapacityReservationsReady` — The selected capacity reservations were resolved.
    - `ValidationSucceeded` — The EC2NodeClass configuration passed validation.
    - `PlacementGroupReady` — The selected placement group was resolved.
  - `status` — The status of a status condition (e.g. the `Ready` condition). For transition metrics this is the state being left.
  - `reason` — The reason dimension. For status-condition metrics it is the condition reason; for event metrics it is the Kubernetes event reason.

## Voluntary Disruption Metrics

### `karpenter_voluntary_disruption_queue_failures_total`
The number of times that an enqueued disruption decision failed. Labeled by disruption method.
- Stability Level: BETA
- Dimensions:
  - `decision` — The disruption decision taken for the candidate(s).
    - `no-op` — No disruption action was taken.
    - `replace` — The candidate(s) were replaced with more efficient capacity.
    - `delete` — The candidate(s) were deleted without replacement.
    - `approved` — The disruption decision was approved for execution.
    - `rejected` — The disruption decision was rejected before execution.
  - `reason` — Why the action was taken. Values are metric-specific: create/delete counters use `provisioned`, `expired`, or `unhealthy`; disruption metrics use the disruption reason such as `underutilized`, `empty`, `drifted`, or `expired`; cloud-provider failure metrics use the provider error reason.
  - `consolidation_type` — The consolidation algorithm that produced the decision.
    - `multi` — Consolidation that considers removing multiple nodes at once.
    - `single` — Consolidation that considers removing a single node.

### `karpenter_voluntary_disruption_failed_validations_total`
Number of candidates that were selected for disruption but failed validation. Labeled by consolidation type.
- Stability Level: ALPHA
- Dimensions:
  - `consolidation_type` — The consolidation algorithm that produced the decision.
    - `multi` — Consolidation that considers removing multiple nodes at once.
    - `single` — Consolidation that considers removing a single node.

### `karpenter_voluntary_disruption_eligible_nodes`
Number of nodes eligible for disruption by Karpenter. Labeled by disruption reason.
- Stability Level: BETA
- Dimensions:
  - `reason` — Why the action was taken. Values are metric-specific: create/delete counters use `provisioned`, `expired`, or `unhealthy`; disruption metrics use the disruption reason such as `underutilized`, `empty`, `drifted`, or `expired`; cloud-provider failure metrics use the provider error reason.

### `karpenter_voluntary_disruption_decisions_total`
Number of disruption decisions performed. Labeled by disruption decision, reason, and consolidation type.
- Stability Level: STABLE
- Dimensions:
  - `decision` — The disruption decision taken for the candidate(s).
    - `no-op` — No disruption action was taken.
    - `replace` — The candidate(s) were replaced with more efficient capacity.
    - `delete` — The candidate(s) were deleted without replacement.
    - `approved` — The disruption decision was approved for execution.
    - `rejected` — The disruption decision was rejected before execution.
  - `reason` — Why the action was taken. Values are metric-specific: create/delete counters use `provisioned`, `expired`, or `unhealthy`; disruption metrics use the disruption reason such as `underutilized`, `empty`, `drifted`, or `expired`; cloud-provider failure metrics use the provider error reason.
  - `consolidation_type` — The consolidation algorithm that produced the decision.
    - `multi` — Consolidation that considers removing multiple nodes at once.
    - `single` — Consolidation that considers removing a single node.

### `karpenter_voluntary_disruption_decisions_by_nodepool_total`
Number of disruption decisions performed by nodepool. Labeled by nodepool name, disruption decision, reason, and consolidation type.
- Stability Level: ALPHA
- Dimensions:
  - `nodepool` — The name of the NodePool that owns the resource.
  - `decision` — The disruption decision taken for the candidate(s).
    - `no-op` — No disruption action was taken.
    - `replace` — The candidate(s) were replaced with more efficient capacity.
    - `delete` — The candidate(s) were deleted without replacement.
    - `approved` — The disruption decision was approved for execution.
    - `rejected` — The disruption decision was rejected before execution.
  - `reason` — Why the action was taken. Values are metric-specific: create/delete counters use `provisioned`, `expired`, or `unhealthy`; disruption metrics use the disruption reason such as `underutilized`, `empty`, `drifted`, or `expired`; cloud-provider failure metrics use the provider error reason.
  - `consolidation_type` — The consolidation algorithm that produced the decision.
    - `multi` — Consolidation that considers removing multiple nodes at once.
    - `single` — Consolidation that considers removing a single node.

### `karpenter_voluntary_disruption_decision_evaluation_duration_seconds`
Duration of the disruption decision evaluation process in seconds. Labeled by method and consolidation type.
- Stability Level: BETA
- Dimensions:
  - `reason` — Why the action was taken. Values are metric-specific: create/delete counters use `provisioned`, `expired`, or `unhealthy`; disruption metrics use the disruption reason such as `underutilized`, `empty`, `drifted`, or `expired`; cloud-provider failure metrics use the provider error reason.
  - `consolidation_type` — The consolidation algorithm that produced the decision.
    - `multi` — Consolidation that considers removing multiple nodes at once.
    - `single` — Consolidation that considers removing a single node.

### `karpenter_voluntary_disruption_consolidation_timeouts_total`
Number of times the Consolidation algorithm has reached a timeout. Labeled by consolidation type.
- Stability Level: BETA
- Dimensions:
  - `consolidation_type` — The consolidation algorithm that produced the decision.
    - `multi` — Consolidation that considers removing multiple nodes at once.
    - `single` — Consolidation that considers removing a single node.

## Scheduler Metrics

### `karpenter_scheduler_unschedulable_pods_count`
The number of unschedulable Pods.
- Stability Level: ALPHA
- Dimensions:
  - `controller` — The name of the controller that emitted the metric.

### `karpenter_scheduler_unfinished_work_seconds`
How many seconds of work has been done that is in progress and hasn't been observed by scheduling_duration_seconds.
- Stability Level: ALPHA
- Dimensions:
  - `controller` — The name of the controller that emitted the metric.
  - `scheduling_id` — A unique identifier for a scheduling simulation run.

### `karpenter_scheduler_scheduling_duration_seconds`
Duration of scheduling simulations used for deprovisioning and provisioning in seconds.
- Stability Level: STABLE
- Dimensions:
  - `controller` — The name of the controller that emitted the metric.

### `karpenter_scheduler_queue_depth`
The number of pods currently waiting to be scheduled.
- Stability Level: BETA
- Dimensions:
  - `controller` — The name of the controller that emitted the metric.
  - `scheduling_id` — A unique identifier for a scheduling simulation run.

### `karpenter_scheduler_pending_pods_by_effective_zone_count`
Pending pods dimensioned by effective zone constraint, or the intersection of pod-level zone signals, volume topology (PVC zones), and topology constraints. Values: specific zone name (e.g., 'us-west-2a'), 'flexible' (multiple zones), or 'none' (no valid intersection).
- Stability Level: ALPHA
- Dimensions:
  - `controller` — The name of the controller that emitted the metric.
  - `zone` — The availability zone of the instance.

### `karpenter_scheduler_ignored_pods_count`
Number of pods ignored during scheduling by Karpenter
- Stability Level: ALPHA

## Nodepools Metrics

### `karpenter_nodepools_usage`
The amount of resources that have been provisioned for a nodepool. Labeled by nodepool name and resource type.
- Stability Level: ALPHA
- Dimensions:
  - `resource_type` — The Kubernetes resource type, e.g. `cpu`, `memory`, `pods`.
  - `nodepool` — The name of the NodePool that owns the resource.

### `karpenter_nodepools_nodes_consuming_budgets`
The number of nodes consuming the budget of a nodepool at a point in time. Labeled by NodePool.
- Stability Level: ALPHA
- Dimensions:
  - `nodepool` — The name of the NodePool that owns the resource.
  - `reason` — Why the action was taken. Values are metric-specific: create/delete counters use `provisioned`, `expired`, or `unhealthy`; disruption metrics use the disruption reason such as `underutilized`, `empty`, `drifted`, or `expired`; cloud-provider failure metrics use the provider error reason.

### `karpenter_nodepools_limit`
Limits specified on the nodepool that restrict the quantity of resources provisioned. Labeled by nodepool name and resource type.
- Stability Level: ALPHA
- Dimensions:
  - `resource_type` — The Kubernetes resource type, e.g. `cpu`, `memory`, `pods`.
  - `nodepool` — The name of the NodePool that owns the resource.

### `karpenter_nodepools_cost_tracker_errors_total`
Number of errors encountered during cost tracking operations. Labeled by nodepool and nodeclaim.
- Stability Level: ALPHA
- Dimensions:
  - `nodepool` — The name of the NodePool that owns the resource.

### `karpenter_nodepools_cost_total`
Total cost of the nodepool from Karpenter's perspective. Units are determined by the cloud provider. Not an authoritative source for billing. Includes modifications due to NodeOverlays
- Stability Level: ALPHA
- Dimensions:
  - `nodepool` — The name of the NodePool that owns the resource.

### `karpenter_nodepools_allowed_disruptions`
The number of nodes for a given NodePool that can be concurrently disrupting at a point in time. Labeled by NodePool. Note that allowed disruptions can change very rapidly, as new nodes may be created and others may be deleted at any point.
- Stability Level: ALPHA
- Dimensions:
  - `nodepool` — The name of the NodePool that owns the resource.
  - `reason` — Why the action was taken. Values are metric-specific: create/delete counters use `provisioned`, `expired`, or `unhealthy`; disruption metrics use the disruption reason such as `underutilized`, `empty`, `drifted`, or `expired`; cloud-provider failure metrics use the provider error reason.

## Interruption Metrics

### `karpenter_interruption_received_messages_total`
Count of messages received from the SQS queue. Broken down by message type and whether the message was actionable.
- Stability Level: STABLE
- Dimensions:
  - `message_type` — The type of interruption message received from the SQS queue, e.g. `spot_interruption`, `scheduled_change`, `state_change`, `rebalance_recommendation`.

### `karpenter_interruption_message_queue_duration_seconds`
Amount of time an interruption message is on the queue before it is processed by karpenter.
- Stability Level: STABLE

### `karpenter_interruption_instance_status_unhealthy_total`
Count of unique unhealthy instance statuses detected from EC2 DescribeInstanceStatus. Broken down by status check category.
- Stability Level: STABLE
- Dimensions:
  - `category` — The EC2 instance status check category that was detected as unhealthy.

### `karpenter_interruption_deleted_messages_total`
Count of messages deleted from the SQS queue.
- Stability Level: STABLE

## EC2NodeClasses Metrics

### `karpenter_ec2nodeclasses_userdata_bytes`
Size in bytes of the rendered user data (raw, pre-base64) for the EC2NodeClass
- Stability Level: ALPHA
- Dimensions:
  - `nodeclass` — The name of the EC2NodeClass the metric was recorded for.

## Cluster Metrics

### `karpenter_cluster_utilization_percent`
Utilization of allocatable resources by pod requests
- Stability Level: ALPHA
- Dimensions:
  - `resource_type` — The Kubernetes resource type, e.g. `cpu`, `memory`, `pods`.

## Cluster State Metrics

### `karpenter_cluster_state_unsynced_time_seconds`
The time for which cluster state is not synced
- Stability Level: STABLE

### `karpenter_cluster_state_synced`
Returns 1 if cluster state is synced and 0 otherwise. Synced checks that nodeclaims and nodes that are stored in the APIServer have the same representation as Karpenter's cluster state
- Stability Level: STABLE

### `karpenter_cluster_state_node_count`
Current count of nodes in cluster state
- Stability Level: STABLE

## Cloudprovider Metrics

### `karpenter_cloudprovider_instance_type_offering_price_estimate`
Instance type offering estimated hourly price used when making informed decisions on node cost calculation, based on instance type, capacity type, and zone.
- Stability Level: BETA
- Dimensions:
  - `instance_type` — The EC2 instance type, e.g. `m5.large`.
  - `capacity_type` — The capacity type of the instance.
    - `on-demand` — On-demand capacity.
    - `spot` — Spot capacity, which can be reclaimed by the cloud provider.
    - `reserved` — Reserved capacity, backed by a capacity reservation.
  - `zone` — The availability zone of the instance.

### `karpenter_cloudprovider_instance_type_offering_available`
Instance type offering availability, based on instance type, capacity type, and zone
- Stability Level: BETA
- Dimensions:
  - `instance_type` — The EC2 instance type, e.g. `m5.large`.
  - `capacity_type` — The capacity type of the instance.
    - `on-demand` — On-demand capacity.
    - `spot` — Spot capacity, which can be reclaimed by the cloud provider.
    - `reserved` — Reserved capacity, backed by a capacity reservation.
  - `zone` — The availability zone of the instance.

### `karpenter_cloudprovider_instance_type_memory_bytes`
Memory, in bytes, for a given instance type.
- Stability Level: BETA
- Dimensions:
  - `instance_type` — The EC2 instance type, e.g. `m5.large`.

### `karpenter_cloudprovider_instance_type_cpu_cores`
VCPUs cores for a given instance type.
- Stability Level: BETA
- Dimensions:
  - `instance_type` — The EC2 instance type, e.g. `m5.large`.

### `karpenter_cloudprovider_instance_termination_failures_total`
Number of instance termination (TerminateInstances) failures, dimensioned by availability zone and zone ID.
- Stability Level: BETA
- Dimensions:
  - `zone` — The availability zone of the instance.
  - `zone_id` — The availability zone ID of the instance, e.g. `usw2-az1` (stable across accounts, unlike the zone name).

### `karpenter_cloudprovider_instance_launch_failures_total`
Number of instance launch (CreateFleet offering) failures, dimensioned by availability zone, zone ID, capacity type, and launch failure reason.
- Stability Level: BETA
- Dimensions:
  - `zone` — The availability zone of the instance.
  - `zone_id` — The availability zone ID of the instance, e.g. `usw2-az1` (stable across accounts, unlike the zone name).
  - `capacity_type` — The capacity type of the instance.
    - `on-demand` — On-demand capacity.
    - `spot` — Spot capacity, which can be reclaimed by the cloud provider.
    - `reserved` — Reserved capacity, backed by a capacity reservation.
  - `reason` — Why the action was taken. Values are metric-specific: create/delete counters use `provisioned`, `expired`, or `unhealthy`; disruption metrics use the disruption reason such as `underutilized`, `empty`, `drifted`, or `expired`; cloud-provider failure metrics use the provider error reason.

### `karpenter_cloudprovider_errors_total`
Total number of errors returned from CloudProvider calls.
- Stability Level: BETA
- Dimensions:
  - `controller` — The name of the controller that emitted the metric.
  - `method` — The CloudProvider interface method that was called, e.g. `Create`, `Delete`, `Get`, `List`, `GetInstanceTypes`, `IsDrifted`.
  - `provider` — The name of the cloud provider implementation.
  - `error` — The category of error returned by the CloudProvider call.
    - `NodeClaimNotFoundError` — The NodeClaim's backing instance was not found.
    - `NodeClassNotReadyError` — The referenced NodeClass is not yet ready.
    - `InsufficientCapacityError` — The cloud provider had insufficient capacity to fulfill the request.

### `karpenter_cloudprovider_duration_seconds`
Duration of cloud provider method calls. Labeled by the controller, method name and provider.
- Stability Level: BETA
- Dimensions:
  - `controller` — The name of the controller that emitted the metric.
  - `method` — The CloudProvider interface method that was called, e.g. `Create`, `Delete`, `Get`, `List`, `GetInstanceTypes`, `IsDrifted`.
  - `provider` — The name of the cloud provider implementation.

## Cloudprovider Batcher Metrics

### `karpenter_cloudprovider_batcher_batch_time_seconds`
Duration of the batching window per batcher
- Stability Level: BETA
- Dimensions:
  - `batcher` — The name of the request batcher the metric was recorded for, e.g. `create_fleet`, `terminate_instances`.

### `karpenter_cloudprovider_batcher_batch_size`
Size of the request batch per batcher
- Stability Level: BETA
- Dimensions:
  - `batcher` — The name of the request batcher the metric was recorded for, e.g. `create_fleet`, `terminate_instances`.

## Controller Runtime Metrics

### `controller_runtime_terminal_reconcile_errors_total`
Total number of terminal reconciliation errors per controller
- Stability Level: STABLE
- Dimensions:
  - `controller` — The name of the controller that owns the reconcile loop.

### `controller_runtime_reconcile_total`
Total number of reconciliations per controller
- Stability Level: STABLE
- Dimensions:
  - `controller` — The name of the controller that owns the reconcile loop.
  - `result` — The outcome of the reconcile call.
    - `success`
    - `error`
    - `requeue`
    - `requeue_after`

### `controller_runtime_reconcile_timeouts_total`
Total number of reconciliation timeouts per controller
- Stability Level: STABLE
- Dimensions:
  - `controller` — The name of the controller that owns the reconcile loop.

### `controller_runtime_reconcile_time_seconds`
Length of time per reconciliation per controller
- Stability Level: STABLE
- Dimensions:
  - `controller` — The name of the controller that owns the reconcile loop.

### `controller_runtime_reconcile_panics_total`
Total number of reconciliation panics per controller
- Stability Level: STABLE
- Dimensions:
  - `controller` — The name of the controller that owns the reconcile loop.

### `controller_runtime_reconcile_errors_total`
Total number of reconciliation errors per controller
- Stability Level: STABLE
- Dimensions:
  - `controller` — The name of the controller that owns the reconcile loop.

### `controller_runtime_max_concurrent_reconciles`
Maximum number of concurrent reconciles per controller
- Stability Level: STABLE
- Dimensions:
  - `controller` — The name of the controller that owns the reconcile loop.

### `controller_runtime_conversion_webhook_panics_total`
Total number of conversion webhook panics
- Stability Level: STABLE

### `controller_runtime_active_workers`
Number of currently used workers per controller
- Stability Level: STABLE
- Dimensions:
  - `controller` — The name of the controller that owns the reconcile loop.

## Workqueue Metrics

### `workqueue_work_duration_seconds`
How long in seconds processing an item from workqueue takes.
- Stability Level: STABLE
- Dimensions:
  - `name`
  - `controller` — The name of the controller that emitted the metric.

### `workqueue_unfinished_work_seconds`
How many seconds of work has been done that is in progress and hasn't been observed by work_duration. Large values indicate stuck threads. One can deduce the number of stuck threads by observing the rate at which this increases.
- Stability Level: STABLE
- Dimensions:
  - `name`
  - `controller` — The name of the controller that emitted the metric.

### `workqueue_retries_total`
Total number of retries handled by workqueue
- Stability Level: STABLE
- Dimensions:
  - `name`
  - `controller` — The name of the controller that emitted the metric.

### `workqueue_queue_duration_seconds`
How long in seconds an item stays in workqueue before being requested
- Stability Level: STABLE
- Dimensions:
  - `name`
  - `controller` — The name of the controller that emitted the metric.

### `workqueue_longest_running_processor_seconds`
How many seconds has the longest running processor for workqueue been running.
- Stability Level: STABLE
- Dimensions:
  - `name`
  - `controller` — The name of the controller that emitted the metric.

### `workqueue_depth`
Current depth of workqueue by workqueue and priority
- Stability Level: STABLE
- Dimensions:
  - `name`
  - `controller` — The name of the controller that emitted the metric.
  - `priority`

### `workqueue_adds_total`
Total number of adds handled by workqueue
- Stability Level: STABLE
- Dimensions:
  - `name`
  - `controller` — The name of the controller that emitted the metric.

## Termination Metrics

### `operator_termination_duration_seconds`
The amount of time taken by an object to terminate completely.
- Stability Level: DEPRECATED
- Dimensions:
  - `group` — The API group of the object the metric describes, e.g. `karpenter.sh`.
  - `kind` — The Kind of the object the metric describes, e.g. `NodeClaim`.
    - `EC2NodeClass`
    - `NodeClaim`
    - `NodePool`

### `operator_termination_current_time_seconds`
The current amount of time in seconds that an object has been in terminating state.
- Stability Level: DEPRECATED
- Dimensions:
  - `namespace` — The namespace of the object the metric describes.
  - `name` — The name of the object the metric describes.
  - `group` — The API group of the object the metric describes, e.g. `karpenter.sh`.
  - `kind` — The Kind of the object the metric describes, e.g. `NodeClaim`.
    - `EC2NodeClass`
    - `NodeClaim`
    - `NodePool`

## Status Condition Metrics

### `operator_status_condition_transitions_total`
The count of transitions of a given object, type and status.
- Stability Level: DEPRECATED
- Dimensions:
  - `type` — The type dimension. For status-condition metrics it is the status condition type (e.g. `Ready`); for event metrics it is the Kubernetes event type (`Normal` or `Warning`).
    - `SubnetsReady` — The selected subnets were resolved.
    - `SecurityGroupsReady` — The selected security groups were resolved.
    - `AMIsReady` — The selected AMIs were resolved.
    - `InstanceProfileReady` — The instance profile was resolved or created.
    - `CapacityReservationsReady` — The selected capacity reservations were resolved.
    - `ValidationSucceeded` — The EC2NodeClass configuration passed validation.
    - `PlacementGroupReady` — The selected placement group was resolved.
    - `Launched` — The instance backing the NodeClaim has been launched with the cloud provider.
    - `Registered` — The launched node has registered with the cluster.
    - `Initialized` — The registered node has finished initializing and is ready for workloads.
    - `Consolidatable` — The NodeClaim is currently eligible for consolidation.
    - `Drifted` — The NodeClaim has drifted from its desired specification.
    - `Drained` — The node has been drained of pods during termination.
    - `VolumesDetached` — The node's volumes have been detached during termination.
    - `InstanceTerminating` — The backing instance is terminating.
    - `ConsistentStateFound` — A consistent instance state was observed before disruption.
    - `DisruptionReason` — Set while the NodeClaim is being disrupted; records the disruption reason.
    - `NodeClassReady` — The underlying NodeClass was resolved and is reporting as Ready.
    - `NodeRegistrationHealthy` — No misconfiguration is preventing successful node launch/registration.
  - `status` — The status of a status condition (e.g. the `Ready` condition). For transition metrics this is the state being left.
  - `reason` — The reason dimension. For status-condition metrics it is the condition reason; for event metrics it is the Kubernetes event reason.
  - `group` — The API group of the object the metric describes, e.g. `karpenter.sh`.
  - `kind` — The Kind of the object the metric describes, e.g. `NodeClaim`.
    - `EC2NodeClass`
    - `NodeClaim`
    - `NodePool`

### `operator_status_condition_transition_seconds`
The amount of time a condition was in a given state before transitioning. e.g. Alarm := P99(Updated=False) > 5 minutes
- Stability Level: DEPRECATED
- Dimensions:
  - `type` — The type dimension. For status-condition metrics it is the status condition type (e.g. `Ready`); for event metrics it is the Kubernetes event type (`Normal` or `Warning`).
    - `SubnetsReady` — The selected subnets were resolved.
    - `SecurityGroupsReady` — The selected security groups were resolved.
    - `AMIsReady` — The selected AMIs were resolved.
    - `InstanceProfileReady` — The instance profile was resolved or created.
    - `CapacityReservationsReady` — The selected capacity reservations were resolved.
    - `ValidationSucceeded` — The EC2NodeClass configuration passed validation.
    - `PlacementGroupReady` — The selected placement group was resolved.
    - `Launched` — The instance backing the NodeClaim has been launched with the cloud provider.
    - `Registered` — The launched node has registered with the cluster.
    - `Initialized` — The registered node has finished initializing and is ready for workloads.
    - `Consolidatable` — The NodeClaim is currently eligible for consolidation.
    - `Drifted` — The NodeClaim has drifted from its desired specification.
    - `Drained` — The node has been drained of pods during termination.
    - `VolumesDetached` — The node's volumes have been detached during termination.
    - `InstanceTerminating` — The backing instance is terminating.
    - `ConsistentStateFound` — A consistent instance state was observed before disruption.
    - `DisruptionReason` — Set while the NodeClaim is being disrupted; records the disruption reason.
    - `NodeClassReady` — The underlying NodeClass was resolved and is reporting as Ready.
    - `NodeRegistrationHealthy` — No misconfiguration is preventing successful node launch/registration.
  - `status` — The status of a status condition (e.g. the `Ready` condition). For transition metrics this is the state being left.
  - `to_status` — The status a condition transitioned to, for transition metrics.
  - `group` — The API group of the object the metric describes, e.g. `karpenter.sh`.
  - `kind` — The Kind of the object the metric describes, e.g. `NodeClaim`.
    - `EC2NodeClass`
    - `NodeClaim`
    - `NodePool`

### `operator_status_condition_current_status_seconds`
The current amount of time in seconds that a status condition has been in a specific state. Alarm := P99(Updated=Unknown) > 5 minutes
- Stability Level: DEPRECATED
- Dimensions:
  - `namespace` — The namespace of the object the metric describes.
  - `name` — The name of the object the metric describes.
  - `type` — The type dimension. For status-condition metrics it is the status condition type (e.g. `Ready`); for event metrics it is the Kubernetes event type (`Normal` or `Warning`).
    - `SubnetsReady` — The selected subnets were resolved.
    - `SecurityGroupsReady` — The selected security groups were resolved.
    - `AMIsReady` — The selected AMIs were resolved.
    - `InstanceProfileReady` — The instance profile was resolved or created.
    - `CapacityReservationsReady` — The selected capacity reservations were resolved.
    - `ValidationSucceeded` — The EC2NodeClass configuration passed validation.
    - `PlacementGroupReady` — The selected placement group was resolved.
    - `Launched` — The instance backing the NodeClaim has been launched with the cloud provider.
    - `Registered` — The launched node has registered with the cluster.
    - `Initialized` — The registered node has finished initializing and is ready for workloads.
    - `Consolidatable` — The NodeClaim is currently eligible for consolidation.
    - `Drifted` — The NodeClaim has drifted from its desired specification.
    - `Drained` — The node has been drained of pods during termination.
    - `VolumesDetached` — The node's volumes have been detached during termination.
    - `InstanceTerminating` — The backing instance is terminating.
    - `ConsistentStateFound` — A consistent instance state was observed before disruption.
    - `DisruptionReason` — Set while the NodeClaim is being disrupted; records the disruption reason.
    - `NodeClassReady` — The underlying NodeClass was resolved and is reporting as Ready.
    - `NodeRegistrationHealthy` — No misconfiguration is preventing successful node launch/registration.
  - `status` — The status of a status condition (e.g. the `Ready` condition). For transition metrics this is the state being left.
  - `reason` — The reason dimension. For status-condition metrics it is the condition reason; for event metrics it is the Kubernetes event reason.
  - `group` — The API group of the object the metric describes, e.g. `karpenter.sh`.
  - `kind` — The Kind of the object the metric describes, e.g. `NodeClaim`.
    - `EC2NodeClass`
    - `NodeClaim`
    - `NodePool`

### `operator_status_condition_count`
The number of a condition for a given object, type and status. e.g. Alarm := Available=False > 0
- Stability Level: DEPRECATED
- Dimensions:
  - `namespace` — The namespace of the object the metric describes.
  - `name` — The name of the object the metric describes.
  - `type` — The type dimension. For status-condition metrics it is the status condition type (e.g. `Ready`); for event metrics it is the Kubernetes event type (`Normal` or `Warning`).
    - `SubnetsReady` — The selected subnets were resolved.
    - `SecurityGroupsReady` — The selected security groups were resolved.
    - `AMIsReady` — The selected AMIs were resolved.
    - `InstanceProfileReady` — The instance profile was resolved or created.
    - `CapacityReservationsReady` — The selected capacity reservations were resolved.
    - `ValidationSucceeded` — The EC2NodeClass configuration passed validation.
    - `PlacementGroupReady` — The selected placement group was resolved.
    - `Launched` — The instance backing the NodeClaim has been launched with the cloud provider.
    - `Registered` — The launched node has registered with the cluster.
    - `Initialized` — The registered node has finished initializing and is ready for workloads.
    - `Consolidatable` — The NodeClaim is currently eligible for consolidation.
    - `Drifted` — The NodeClaim has drifted from its desired specification.
    - `Drained` — The node has been drained of pods during termination.
    - `VolumesDetached` — The node's volumes have been detached during termination.
    - `InstanceTerminating` — The backing instance is terminating.
    - `ConsistentStateFound` — A consistent instance state was observed before disruption.
    - `DisruptionReason` — Set while the NodeClaim is being disrupted; records the disruption reason.
    - `NodeClassReady` — The underlying NodeClass was resolved and is reporting as Ready.
    - `NodeRegistrationHealthy` — No misconfiguration is preventing successful node launch/registration.
  - `status` — The status of a status condition (e.g. the `Ready` condition). For transition metrics this is the state being left.
  - `reason` — The reason dimension. For status-condition metrics it is the condition reason; for event metrics it is the Kubernetes event reason.
  - `group` — The API group of the object the metric describes, e.g. `karpenter.sh`.
  - `kind` — The Kind of the object the metric describes, e.g. `NodeClaim`.
    - `EC2NodeClass`
    - `NodeClaim`
    - `NodePool`

## Client Go Metrics

### `client_go_request_total`
Number of HTTP requests, partitioned by status code and method.
- Stability Level: STABLE

### `client_go_request_duration_seconds`
Request latency in seconds. Broken down by verb, group, version, kind, and subresource.
- Stability Level: STABLE

## AWS SDK Go Metrics

### `aws_sdk_go_request_total`
The total number of AWS SDK Go requests
- Stability Level: STABLE
- Dimensions:
  - `service` — The AWS service the request was made to, e.g. `EC2`.
  - `action` — The AWS API operation invoked, e.g. `DescribeSubnets`.
  - `code` — The HTTP status code of the response, e.g. `200`, `503`.

### `aws_sdk_go_request_retry_count`
The total number of AWS SDK Go retry attempts per request
- Stability Level: STABLE
- Dimensions:
  - `service` — The AWS service the request was made to, e.g. `EC2`.
  - `action` — The AWS API operation invoked, e.g. `DescribeSubnets`.
  - `code` — The HTTP status code of the response, e.g. `200`, `503`.

### `aws_sdk_go_request_duration_seconds`
Latency of AWS SDK Go requests
- Stability Level: STABLE
- Dimensions:
  - `service` — The AWS service the request was made to, e.g. `EC2`.
  - `action` — The AWS API operation invoked, e.g. `DescribeSubnets`.
  - `code` — The HTTP status code of the response, e.g. `200`, `503`.

### `aws_sdk_go_request_attempt_total`
The total number of AWS SDK Go request attempts
- Stability Level: STABLE
- Dimensions:
  - `service` — The AWS service the request was made to, e.g. `EC2`.
  - `action` — The AWS API operation invoked, e.g. `DescribeSubnets`.
  - `code` — The HTTP status code of the response, e.g. `200`, `503`.

### `aws_sdk_go_request_attempt_duration_seconds`
Latency of AWS SDK Go request attempts
- Stability Level: STABLE
- Dimensions:
  - `service` — The AWS service the request was made to, e.g. `EC2`.
  - `action` — The AWS API operation invoked, e.g. `DescribeSubnets`.
  - `code` — The HTTP status code of the response, e.g. `200`, `503`.

## Leader Election Metrics

### `leader_election_slowpath_total`
Total number of slow path exercised in renewing leader leases. 'name' is the string used to identify the lease. Please make sure to group by name.
- Stability Level: STABLE
- Dimensions:
  - `name`

### `leader_election_master_status`
Gauge of if the reporting system is master of the relevant lease, 0 indicates backup, 1 indicates master. 'name' is the string used to identify the lease. Please make sure to group by name.
- Stability Level: STABLE
- Dimensions:
  - `name`

