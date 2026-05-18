---
title: "AWS Provider Scope"
linkTitle: "AWS Provider Scope"
weight: 15
description: >
  What is and isn't in scope for the AWS provider
---

In addition to the [core Karpenter scope guidelines](https://github.com/kubernetes-sigs/karpenter/blob/main/SCOPE.md), the following categories are out of scope for the AWS provider specifically.

## Provider-Neutral Features in the AWS Provider

Features that apply to all cloud providers belong in [kubernetes-sigs/karpenter](https://github.com/kubernetes-sigs/karpenter), not in the AWS provider. The AWS provider implements cloud-specific behavior only.

## Passthrough Configuration That Bypasses Karpenter's Abstraction

The AWS provider presents an opinionated abstraction over launch templates and EC2 configuration. Bypassing this abstraction can leave Karpenter unaware of the underlying node capabilities and their Kubernetes representation, leading to incorrect launch or scheduling decisions.

## How to Check if Your Idea Fits

Before opening a PR:

- Search [open issues](https://github.com/aws/karpenter-provider-aws/issues) and [designs/](https://github.com/aws/karpenter-provider-aws/tree/main/designs) for prior discussion
- Ask: does this add API surface? If yes, can the problem be solved without it?
- Ask: does this solve a problem for the broad user base, or for a specific operational scenario?
- Ask: does this duplicate functionality that belongs in another system (scheduler, cost tools, monitoring)?
- When in doubt, open an issue describing the problem and discuss before writing code
