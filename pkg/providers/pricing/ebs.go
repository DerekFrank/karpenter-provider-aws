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

package pricing

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	pricingtypes "github.com/aws/aws-sdk-go-v2/service/pricing/types"
	"github.com/samber/lo"
	"go.uber.org/multierr"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// HoursPerMonth is the number of hours AWS uses to convert monthly EBS rates into hourly rates.
const HoursPerMonth = 730.0

var initialEBSPrices = lo.Assign(InitialEBSPricesAWS, InitialEBSPricesUSGov)

// EBSPrice holds the monthly EBS rates for a single volume type.
type EBSPrice struct {
	// GBMonth is the price per provisioned GiB-month of storage.
	GBMonth float64
	// IOPSMonth is the price per provisioned IOPS-month. For io2, this is the price of the first 32,000 IOPS.
	IOPSMonth float64
	// IOPSMonthTier2 is the io2 price per provisioned IOPS-month between 32,001 and 64,000 IOPS.
	IOPSMonthTier2 float64
	// IOPSMonthTier3 is the io2 price per provisioned IOPS-month above 64,000 IOPS.
	IOPSMonthTier3 float64
	// ThroughputMonth is the price per provisioned MiB/s-month.
	ThroughputMonth float64
}

// Baseline performance included in the gp3 storage price. Only performance provisioned above these is billed.
// https://aws.amazon.com/ebs/pricing/
const (
	gp3BaselineIOPS       = 3000
	gp3BaselineThroughput = 125
	io2Tier2StartIOPS     = 32000
	io2Tier3StartIOPS     = 64000
)

// EBSVolumePrice returns the hourly price of an EBS volume with the given type, size (GiB), provisioned IOPS, and
// provisioned throughput (MiB/s), returning false if there is no known pricing for the volume type.
func (p *DefaultProvider) EBSVolumePrice(volumeType string, sizeGiB int64, iops int64, throughput int64) (float64, bool) {
	p.muEBS.RLock()
	defer p.muEBS.RUnlock()
	price, ok := p.ebsPrices[volumeType]
	if !ok {
		return 0, false
	}
	return VolumePrice(volumeType, price, sizeGiB, iops, throughput), true
}

// VolumePrice returns the hourly price of an EBS volume given the monthly rates for its volume type.
func VolumePrice(volumeType string, price EBSPrice, sizeGiB int64, iops int64, throughput int64) float64 {
	monthly := float64(sizeGiB) * price.GBMonth
	switch volumeType {
	case string(ec2types.VolumeTypeGp3):
		monthly += float64(max(iops-gp3BaselineIOPS, 0)) * price.IOPSMonth
		monthly += float64(max(throughput-gp3BaselineThroughput, 0)) * price.ThroughputMonth
	case string(ec2types.VolumeTypeIo1):
		monthly += float64(iops) * price.IOPSMonth
	case string(ec2types.VolumeTypeIo2):
		monthly += float64(min(iops, io2Tier2StartIOPS)) * price.IOPSMonth
		monthly += float64(min(max(iops-io2Tier2StartIOPS, 0), io2Tier3StartIOPS-io2Tier2StartIOPS)) * price.IOPSMonthTier2
		monthly += float64(max(iops-io2Tier3StartIOPS, 0)) * price.IOPSMonthTier3
	}
	return monthly / HoursPerMonth
}

// EBSPrices returns a copy of the known EBS rates for each volume type.
func (p *DefaultProvider) EBSPrices() map[string]EBSPrice {
	p.muEBS.RLock()
	defer p.muEBS.RUnlock()
	return lo.Assign(p.ebsPrices)
}

// updateEBSPricing fetches EBS volume pricing from the pricing API. Volume types missing from the response keep their
// previous rates, so a partial or empty response never removes pricing.
func (p *DefaultProvider) updateEBSPricing(ctx context.Context) error {
	var errs error
	fetched := map[string]EBSPrice{}
	for _, productFamily := range []string{"Storage", "System Operation", "Provisioned Throughput"} {
		prices, err := p.fetchEBSPricing(ctx, productFamily)
		if err != nil {
			errs = multierr.Append(errs, err)
			continue
		}
		for volumeType, price := range prices {
			fetched[volumeType] = mergeEBSPrice(fetched[volumeType], price)
		}
	}
	if errs != nil {
		return fmt.Errorf("retrieving ebs pricing data, %w", errs)
	}
	// A volume type is only usable if we know its storage price; IOPS and throughput are additive.
	fetched = lo.PickBy(fetched, func(_ string, price EBSPrice) bool { return price.GBMonth > 0 })
	if len(fetched) == 0 {
		return nil
	}

	p.muEBS.Lock()
	defer p.muEBS.Unlock()
	p.ebsPrices = lo.Assign(p.ebsPrices, fetched)
	if p.cm.HasChanged("ebs-prices", p.ebsPrices) {
		log.FromContext(ctx).WithValues("volume-type-count", len(p.ebsPrices)).V(1).Info("updated ebs pricing")
	}
	return nil
}

func (p *DefaultProvider) fetchEBSPricing(ctx context.Context, productFamily string) (map[string]EBSPrice, error) {
	input := &pricing.GetProductsInput{
		Filters: []pricingtypes.Filter{
			{
				Field: aws.String("regionCode"),
				Type:  "TERM_MATCH",
				Value: aws.String(p.region),
			},
			{
				Field: aws.String("serviceCode"),
				Type:  "TERM_MATCH",
				Value: aws.String("AmazonEC2"),
			},
			{
				Field: aws.String("productFamily"),
				Type:  "TERM_MATCH",
				Value: aws.String(productFamily),
			},
		},
		ServiceCode: aws.String("AmazonEC2"),
		MaxResults:  lo.ToPtr[int32](100),
	}
	prices := map[string]EBSPrice{}
	paginator := pricing.NewGetProductsPaginator(p.pricing, input)
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("getting ebs pricing data, %w", err)
		}
		for volumeType, price := range p.ebsPage(ctx, output) {
			prices[volumeType] = mergeEBSPrice(prices[volumeType], price)
		}
	}
	return prices, nil
}

// ebsPage parses the EBS volume rates out of a page of pricing API results. Products that aren't EBS volume storage,
// IOPS, or throughput (e.g. snapshots, or instance pricing) are ignored.
//
//nolint:gocyclo
func (p *DefaultProvider) ebsPage(ctx context.Context, output *pricing.GetProductsOutput) map[string]EBSPrice {
	type priceItem struct {
		Product struct {
			Attributes struct {
				VolumeAPIName string `json:"volumeApiName"`
				UsageType     string `json:"usagetype"`
			}
		}
		Terms struct {
			OnDemand map[string]struct {
				PriceDimensions map[string]struct {
					Unit         string
					PricePerUnit map[string]string
				}
			}
		}
	}
	currency := lo.Ternary(strings.HasPrefix(p.region, "cn-"), "CNY", "USD")
	result := map[string]EBSPrice{}
	for _, outer := range output.PriceList {
		pItem := &priceItem{}
		if err := json.Unmarshal([]byte(outer), pItem); err != nil {
			log.FromContext(ctx).Error(err, "failed unmarshaling ebs pricing data")
			continue
		}
		volumeType := pItem.Product.Attributes.VolumeAPIName
		// Usage types are prefixed with a region code outside of us-east-1 (e.g. USW2-EBS:VolumeUsage.gp3)
		_, usageType, found := strings.Cut(pItem.Product.Attributes.UsageType, "EBS:")
		if volumeType == "" || !found {
			continue
		}
		for _, term := range pItem.Terms.OnDemand {
			for _, dimension := range term.PriceDimensions {
				price, err := strconv.ParseFloat(dimension.PricePerUnit[currency], 64)
				if err != nil || price == 0 {
					continue
				}
				current := result[volumeType]
				switch {
				case strings.HasPrefix(usageType, "VolumeUsage"):
					current.GBMonth = price
				case strings.HasPrefix(usageType, "VolumeP-IOPS"):
					switch {
					case strings.HasSuffix(usageType, ".tier2"):
						current.IOPSMonthTier2 = price
					case strings.HasSuffix(usageType, ".tier3"):
						current.IOPSMonthTier3 = price
					default:
						current.IOPSMonth = price
					}
				case strings.HasPrefix(usageType, "VolumeP-Throughput"):
					// gp3 throughput is published per GiBps-month, but provisioned in MiB/s
					if strings.HasPrefix(dimension.Unit, "GiBps") {
						price /= 1024
					}
					current.ThroughputMonth = price
				default:
					continue
				}
				result[volumeType] = current
			}
		}
	}
	return result
}

// mergeEBSPrice combines two partial sets of rates for the same volume type, preferring the non-zero rates in b.
func mergeEBSPrice(a, b EBSPrice) EBSPrice {
	pick := func(x, y float64) float64 { return lo.Ternary(y != 0, y, x) }
	return EBSPrice{
		GBMonth:         pick(a.GBMonth, b.GBMonth),
		IOPSMonth:       pick(a.IOPSMonth, b.IOPSMonth),
		IOPSMonthTier2:  pick(a.IOPSMonthTier2, b.IOPSMonthTier2),
		IOPSMonthTier3:  pick(a.IOPSMonthTier3, b.IOPSMonthTier3),
		ThroughputMonth: pick(a.ThroughputMonth, b.ThroughputMonth),
	}
}

func staticEBSPricing(region string) map[string]EBSPrice {
	if prices, ok := initialEBSPrices[region]; ok {
		return prices
	}
	// Static EBS prices are in USD. Rather than mixing currencies with CNY instance prices, the China partition has
	// no static EBS pricing and relies on the pricing API.
	if strings.HasPrefix(region, "cn-") {
		return map[string]EBSPrice{}
	}
	return initialEBSPrices["us-east-1"]
}
