---
title: "Monitoring Amazon EC2 API Usage"
linkTitle: "Monitoring Amazon EC2 API Usage"
description: >
  Monitor Karpenter's Amazon EC2 API call volume and request throttling, and keep it within your account's request-rate limits
---

Karpenter calls the Amazon EC2 API to discover and manage the infrastructure it provisions.
The volume of these calls is not fixed: it scales with the number of `EC2NodeClass`es and clusters
you run, the intervals at which Karpenter refreshes cached data, and the Karpenter version you run.
Amazon EC2 enforces per-account, per-Region request-rate limits, so a large enough fleet can generate
enough requests to be throttled.

When Amazon EC2 throttles Karpenter's `Describe*` calls, Karpenter's cached view of your subnets and
security groups becomes stale, which slows the propagation of subnet and security group configuration
changes and makes it more likely that Karpenter launches a node in a subnet that no longer has
sufficient available IP addresses (pods scheduled onto that node then remain in `ContainerCreating`).

This task describes what Amazon EC2 APIs Karpenter calls, how to observe your call volume and
throttling, how to compare that volume against your account's request-rate limits, and how to reduce
and bound it.

## What Amazon EC2 APIs Karpenter calls and why

Karpenter calls Amazon EC2 in two broad situations:

* **Provisioning and terminating nodes.** When Karpenter launches or removes capacity it calls APIs
  such as `CreateFleet`, `RunInstances`, `DescribeInstances`, and `TerminateInstances`. This volume
  scales with how often your cluster scales up and down.
* **Resolving and refreshing `EC2NodeClass` configuration.** For each `EC2NodeClass`, Karpenter
  resolves the subnets, security groups, and AMIs it selects by calling `DescribeSubnets`,
  `DescribeSecurityGroups`, and `DescribeImages`, and resolves instance types with calls such as
  `DescribeInstanceTypes` and `DescribeInstanceTypeOfferings`. Karpenter caches these results and
  refreshes them on an interval.

The refresh volume is the part that scales with the size of your configuration rather than with
cluster activity. Karpenter caches these results **per `EC2NodeClass`** rather than per distinct
selector, so two `EC2NodeClass`es that select the same subnets, security groups, or AMIs do not share
a cached result — each performs and refreshes its own lookup. As a result, the number of
`DescribeSubnets`, `DescribeSecurityGroups`, and `DescribeImages` calls scales with:

* the **number of `EC2NodeClass`es** you run,
* the **number of clusters** in the account and Region, and
* the **refresh interval** — by default Karpenter refreshes each `EC2NodeClass`'s cached data once per
  minute.

{{% alert title="Note" color="primary" %}}
Because this volume scales with the number of `EC2NodeClass`es and clusters — not with the number of
distinct selectors — an account that consolidates a very large number of clusters can generate
substantially more `Describe*` traffic than the number of unique selections would suggest. Per-workload
request volume can also change between Karpenter versions, so validate it when upgrading (see
[When to watch it](#when-to-watch-it)).
{{% /alert %}}

## How to observe call volume and throttling

### Karpenter metrics

Karpenter exposes Prometheus metrics (by default at `:8080/metrics`, configurable via `METRICS_PORT`;
see the [Metrics reference]({{< relref "../reference/metrics" >}})). The AWS SDK request metrics are
the most direct measure of Karpenter's Amazon EC2 call volume and throttling. They are labeled by
`service` (for example, `EC2`), `action` (the API operation, for example `DescribeSubnets`), and
`code` (the HTTP status code — `200` for success, and `503` for the `RequestLimitExceeded`
throttling response):

* `aws_sdk_go_request_total` — total AWS SDK requests, by `service`, `action`, and `code`.
* `aws_sdk_go_request_attempt_total` — total request attempts (a single request may make multiple
  attempts when retried).
* `aws_sdk_go_request_retry_count` — number of retry attempts per request. Sustained retries are an
  early indicator of throttling, because the AWS SDK retries throttled requests before they surface
  as an error.

For example, to graph Karpenter's Amazon EC2 request rate by operation:

```
sum by (action) (rate(aws_sdk_go_request_total{service="EC2"}[5m]))
```

To graph the throttled fraction of Karpenter's Amazon EC2 requests:

```
sum(rate(aws_sdk_go_request_total{service="EC2", code="503"}[5m]))
  / sum(rate(aws_sdk_go_request_total{service="EC2"}[5m]))
```

Karpenter's cloud provider wrapper also exposes:

* `karpenter_cloudprovider_errors_total` — errors returned from cloud provider calls, labeled by
  controller, method, and provider.
* `karpenter_cloudprovider_duration_seconds` — duration of cloud provider method calls.

{{% alert title="Note" color="primary" %}}
Karpenter does not emit a dedicated log line for throttling. It detects the Amazon EC2
`RequestLimitExceeded` error programmatically and, where appropriate, surfaces it as an `EC2NodeClass`
or `NodeClaim` status reason (`RequestLimitExceeded` / "Request limit exceeded") rather than as a log
message. Use the metrics above, or CloudTrail, to observe throttling directly.
{{% /alert %}}

### CloudTrail

Karpenter sets a User-Agent of `karpenter.sh-<version>` on its AWS SDK clients, so you can attribute
Amazon EC2 API events to Karpenter in CloudTrail by filtering `userAgent` for a value that begins with
`karpenter.sh-`. Look at the `Describe*` events (such as `DescribeSubnets` and
`DescribeSecurityGroups`) to see call volume, and inspect the `errorCode` field for
`Client.RequestLimitExceeded` to see throttled calls. CloudTrail is useful for attributing volume to
Karpenter across an account, but it is not intended for high-resolution rate monitoring — use the
metrics above for that.

### CloudWatch and Service Quotas

Amazon EC2 does not publish a general-purpose CloudWatch metric for API call rate or throttling.
To compare your usage against your limits, use Service Quotas (see below), which can graph your
applied quota values, and rely on Karpenter's metrics and CloudTrail for observed call volume and
throttling.

## How to compare against your account's Amazon EC2 API request-rate limits

Amazon EC2 API request-rate limits are enforced **per account, per Region**, and are independent for
different groups of actions (for example, the non-mutating `Describe*` actions are limited separately
from mutating actions). When your request rate exceeds the limit, Amazon EC2 rejects the excess
requests with the `RequestLimitExceeded` error (HTTP 503).

* Review your account's request-rate limits with
  [Service Quotas](https://docs.aws.amazon.com/servicequotas/latest/userguide/intro.html) for Amazon
  EC2 in each Region where you run Karpenter, and compare them against the request rate you observe
  from the metrics and CloudTrail above.
* Request an increase through Service Quotas or AWS Support if your steady-state Karpenter request
  rate is close to, or exceeds, your limits.

Because these limits are per account and per Region, the combined request volume of every cluster in
an account competes for the same limits.

## How to reduce and bound call volume

### Increase Karpenter's refresh intervals

Karpenter refreshes each `EC2NodeClass`'s cached subnet, security group, and AMI data from Amazon EC2
on an interval. Recent Karpenter versions expose some of these intervals as settings so you can reduce
the associated `Describe*` call volume:

* `SUBNET_REFRESH_INTERVAL` — how often subnet data is refreshed (bounds `DescribeSubnets`).
* `AMI_REFRESH_INTERVAL` — how often AMI data is refreshed (bounds `DescribeImages`).

Increasing an interval reduces that call's steady-state rate proportionally — for example, changing an
interval from `1m` to `5m` reduces that call's rate by roughly 5x. The trade-off is staleness: a
longer interval means Karpenter takes longer to observe changes (such as a subnet's available IP
capacity, or a new AMI).

{{% alert title="Feature availability" color="warning" %}}
The set of configurable refresh intervals has expanded over time and differs by version. Consult the
[Settings reference]({{< relref "../reference/settings" >}}) **for your version** to see which
refresh-interval settings (for example `SUBNET_REFRESH_INTERVAL`, `AMI_REFRESH_INTERVAL`, and in some
versions a security group refresh interval) are available and their defaults and minimums. Older
versions may not expose any of them.
{{% /alert %}}

### Consolidate `EC2NodeClass`es that use identical selectors

Because Karpenter refreshes subnet, security group, and AMI data per `EC2NodeClass`, running many
`EC2NodeClass`es that select the same resources multiplies the refresh volume without providing
distinct configuration. Where the configuration is genuinely identical, consolidating onto fewer
`EC2NodeClass`es reduces the number of independent refresh lookups. This is the primary lever for the
`Describe*` calls that do not have a configurable refresh interval in your version.

### Distribute clusters across multiple accounts

Because request-rate limits are per account and per Region, concentrating a very large number of
clusters in a single account concentrates all of their Amazon EC2 request volume against that one
account's limits. Distributing clusters across multiple accounts spreads the volume across multiple
accounts' limits. See the multi-account guidance in the
[AWS Well-Architected Framework](https://docs.aws.amazon.com/wellarchitected/latest/framework/welcome.html).

## When to watch it

The Amazon EC2 request volume a given fleet generates can change between Karpenter versions, so the
most important times to validate it are:

* **When upgrading Karpenter**, since a new version may change how often, or how many, Amazon EC2
  requests each `EC2NodeClass` makes.
* **When rolling out configuration or version changes across a large fleet**, since the change is
  multiplied across every cluster and `EC2NodeClass`.

In both cases, validate your Amazon EC2 call volume against your account's request-rate limits during
qualification, before a broad rollout — for example, roll out to a subset of clusters, observe the
metrics and CloudTrail above, and confirm you remain within your limits before proceeding.
