package aws

import (
	"context"

	"github.com/infracost/infracost/internal/resources"
	"github.com/infracost/infracost/internal/schema"
	"github.com/infracost/infracost/internal/usage/aws"

	"github.com/shopspring/decimal"
)

type LambdaFunction struct {
	Address    string
	Region     string
	Name       string
	MemorySize int64

	RequestDurationMS *float64 `infracost_usage:"request_duration_ms"`
	MonthlyRequests   *float64 `infracost_usage:"monthly_requests"`
}

var LambdaFunctionUsageSchema = []*schema.UsageSchemaItem{
	{Key: "request_duration_ms", DefaultValue: 0, ValueType: schema.Float64},
	{Key: "monthly_requests", DefaultValue: 0, ValueType: schema.Float64},
}

func (a *LambdaFunction) PopulateUsage(u *schema.UsageData) {
	resources.PopulateArgsWithUsage(a, u)
}

func (a *LambdaFunction) BuildResource() *schema.Resource {
	memorySize := decimal.NewFromInt(a.MemorySize)

	averageRequestDuration := decimal.NewFromInt(1)
	if a.RequestDurationMS != nil {
		averageRequestDuration = decimal.NewFromFloat(*a.RequestDurationMS)
	}

	var monthlyRequests *decimal.Decimal
	var gbSeconds *decimal.Decimal

	if a.MonthlyRequests != nil {
		monthlyRequests = decimalPtr(decimal.NewFromFloat(*a.MonthlyRequests))
		gbSeconds = decimalPtr(calculateGBSeconds(memorySize, averageRequestDuration, *monthlyRequests))
	}

	estimate := func(ctx context.Context, values map[string]interface{}) error {
		inv, err := aws.LambdaGetInvocations(ctx, a.Region, a.Name)
		if err != nil {
			return err
		}
		values["monthly_requests"] = inv
		dur, err := aws.LambdaGetDurationAvg(ctx, a.Region, a.Name)
		if err != nil {
			return err
		}
		values["request_duration_ms"] = dur
		return nil
	}

	return &schema.Resource{
		Name:        a.Address,
		UsageSchema: LambdaFunctionUsageSchema,
		CostComponents: []*schema.CostComponent{
			{
				Name:            "Requests",
				Unit:            "1M requests",
				UnitMultiplier:  decimal.NewFromInt(1000000),
				MonthlyQuantity: monthlyRequests,
				ProductFilter: &schema.ProductFilter{
					VendorName:    strPtr("aws"),
					Region:        strPtr(a.Region),
					Service:       strPtr("AWSLambda"),
					ProductFamily: strPtr("Serverless"),
					AttributeFilters: []*schema.AttributeFilter{
						{Key: "group", Value: strPtr("AWS-Lambda-Requests")},
						{Key: "usagetype", ValueRegex: strPtr("/Request/")},
					},
				},
			},
			{
				Name:            "Duration",
				Unit:            "GB-seconds",
				UnitMultiplier:  decimal.NewFromInt(1),
				MonthlyQuantity: gbSeconds,
				ProductFilter: &schema.ProductFilter{
					VendorName:    strPtr("aws"),
					Region:        strPtr(a.Region),
					Service:       strPtr("AWSLambda"),
					ProductFamily: strPtr("Serverless"),
					AttributeFilters: []*schema.AttributeFilter{
						{Key: "group", Value: strPtr("AWS-Lambda-Duration")},
						{Key: "usagetype", ValueRegex: strPtr("/GB-Second/")},
					},
				},
			},
		},
		EstimateUsage: estimate,
	}
}

func calculateGBSeconds(memorySize decimal.Decimal, averageRequestDuration decimal.Decimal, monthlyRequests decimal.Decimal) decimal.Decimal {
	gb := memorySize.Div(decimal.NewFromInt(1024))
	seconds := averageRequestDuration.Ceil().Div(decimal.NewFromInt(1000)) // Round up to closest 1ms and convert to seconds
	return monthlyRequests.Mul(gb).Mul(seconds)
}
